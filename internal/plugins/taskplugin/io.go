package taskplugin

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/term"
	"io"
	"mesos-cli/internal/mesos"
	"mesos-cli/internal/recordio"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"
)

type taskIO struct {
	plugin         *TaskPlugin
	task           mesos.Task
	agentURL       string
	containerID    *mesos.ContainerIDValue
	stdin          io.Reader
	stdout, stderr io.Writer
	outputReady    chan struct{}
}

func (p *TaskPlugin) resolveTask(taskID string, stdin io.Reader, stdout, stderr io.Writer) (*taskIO, error) {
	tasks, err := p.client.Tasks(url.Values{"task_id": {taskID}})
	if err != nil {
		return nil, fmt.Errorf("Unable to get task with ID %s: %v", taskID, err)
	}
	matches := []mesos.Task{}
	for _, task := range tasks {
		if task.ID == taskID && task.State == "TASK_RUNNING" {
			matches = append(matches, task)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("Unable to find running task '%s'", taskID)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("More than one task matching id '%s'", taskID)
	}
	task := matches[0]
	containerID, err := mesos.ContainerID(task)
	if err != nil {
		return nil, fmt.Errorf("Could not get container ID of task '%s': %v", taskID, err)
	}
	address, err := p.client.AgentAddress(task.SlaveID)
	if err != nil {
		return nil, err
	}
	scheme := "http://"
	if p.config.AgentSSL() {
		scheme = "https://"
	}
	return &taskIO{plugin: p, task: task, agentURL: scheme + address + "/api/v1", containerID: containerID, stdin: stdin, stdout: stdout, stderr: stderr}, nil
}
func (p *TaskPlugin) attach(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	noStdin := slices.Contains(args, "--no-stdin")
	filtered := []string{}
	for _, arg := range args {
		if arg != "--no-stdin" {
			filtered = append(filtered, arg)
		}
	}
	if len(filtered) < 1 {
		return 1, fmt.Errorf("missing <task-id>")
	}
	session, err := p.resolveTask(filtered[0], stdin, stdout, stderr)
	if err != nil {
		return 1, err
	}
	if !noStdin {
		return session.attachInteractive()
	}
	if err := session.output("ATTACH_CONTAINER_OUTPUT", map[string]any{"container_id": session.containerID}, false); err != nil {
		return 1, err
	}
	if session.containerID.Parent != nil {
		return session.wait()
	}
	return 0, nil
}
func (p *TaskPlugin) exec(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	interactive := false
	tty := false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-i", "--interactive":
			interactive = true
		case "-t", "--tty":
			tty = true
		case "-it", "-ti":
			interactive = true
			tty = true
		default:
			return 1, fmt.Errorf("unknown option: %s", args[0])
		}
		args = args[1:]
	}
	if len(args) < 2 {
		return 1, fmt.Errorf("missing <task-id> or <command>")
	}
	session, err := p.resolveTask(args[0], stdin, stdout, stderr)
	if err != nil {
		return 1, err
	}
	session.containerID = &mesos.ContainerIDValue{Value: newUUID(), Parent: session.containerID}
	if interactive {
		return session.execInteractive(args[1], args[2:], tty)
	}
	payload := map[string]any{"container_id": session.containerID, "command": map[string]any{"value": args[1], "arguments": append([]string{args[1]}, args[2:]...), "shell": false}}
	if tty {
		payload["container"] = map[string]any{"type": "MESOS", "tty_info": map[string]any{}}
	}
	if err := session.output("LAUNCH_NESTED_CONTAINER_SESSION", payload, false); err != nil {
		return 1, err
	}
	return session.wait()
}
func newUUID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(raw[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}
func (s *taskIO) request(message map[string]any, accept string, body io.Reader, streaming bool) (*http.Response, error) {
	if body == nil {
		data, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(http.MethodPost, s.agentURL, body)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("Content-Type", "application/json")
	if user, secret, ok := s.plugin.config.AgentAuthentication(); ok {
		req.SetBasicAuth(user, secret)
	}
	client := s.plugin.http
	if streaming {
		client = s.plugin.stream
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s: %s", resp.Status, string(data))
	}
	return resp, nil
}
func (s *taskIO) output(kind string, payload map[string]any, interactive bool) error {
	key := strings.ToLower(kind)
	message := map[string]any{"type": kind, key: payload}
	resp, err := s.request(message, "application/recordio", nil, interactive)
	if err != nil {
		return err
	}
	if s.outputReady != nil {
		close(s.outputReady)
		s.outputReady = nil
	}
	defer resp.Body.Close()
	decoder := recordio.NewDecoder()
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			records, err := decoder.Decode(buffer[:n])
			if err != nil {
				return fmt.Errorf("Error parsing output stream: %v", err)
			}
			for _, record := range records {
				var envelope struct {
					Type string `json:"type"`
					Data struct {
						Type string `json:"type"`
						Data string `json:"data"`
					} `json:"data"`
				}
				if err := json.Unmarshal(record, &envelope); err != nil {
					return err
				}
				if envelope.Type != "DATA" {
					continue
				}
				data, err := base64.StdEncoding.DecodeString(envelope.Data.Data)
				if err != nil {
					return err
				}
				switch envelope.Data.Type {
				case "STDOUT":
					if _, err = s.stdout.Write(data); err != nil {
						return err
					}
				case "STDERR":
					if _, err = s.stderr.Write(data); err != nil {
						return err
					}
				default:
					return fmt.Errorf("Unsupported data type in output stream")
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return nil
}
func (s *taskIO) wait() (int, error) {
	message := map[string]any{"type": "WAIT_CONTAINER", "wait_container": map[string]any{"container_id": s.containerID}}
	resp, err := s.request(message, "application/json", nil, false)
	if err != nil {
		return 1, fmt.Errorf("Error waiting for command to complete: %v", err)
	}
	defer resp.Body.Close()
	var result struct {
		Wait struct {
			ExitStatus int `json:"exit_status"`
		} `json:"wait_container"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 1, err
	}
	status := syscall.WaitStatus(result.Wait.ExitStatus)
	if status.Signaled() {
		return 128 + int(status.Signal()), nil
	}
	return status.ExitStatus(), nil
}

func (s *taskIO) attachInteractive() (int, error) {
	return s.runInteractive("ATTACH_CONTAINER_OUTPUT", map[string]any{"container_id": s.containerID}, true, false)
}

func (s *taskIO) execInteractive(command string, args []string, ttyEnabled bool) (int, error) {
	payload := map[string]any{"container_id": s.containerID, "command": map[string]any{"value": command, "arguments": append([]string{command}, args...), "shell": false}}
	if ttyEnabled {
		payload["container"] = map[string]any{"type": "MESOS", "tty_info": map[string]any{}}
	}
	return s.runInteractive("LAUNCH_NESTED_CONTAINER_SESSION", payload, ttyEnabled, true)
}

func (s *taskIO) runInteractive(kind string, payload map[string]any, ttyEnabled, wait bool) (int, error) {
	restore, resize, err := s.prepareTerminal(ttyEnabled)
	if err != nil {
		return 1, err
	}
	defer restore()
	s.outputReady = make(chan struct{})
	outputDone := make(chan error, 1)
	go func() { outputDone <- s.output(kind, payload, true) }()
	<-s.outputReady
	if err := s.streamInput(resize, ttyEnabled); err != nil {
		return 1, err
	}
	if err := <-outputDone; err != nil {
		return 1, err
	}
	if wait {
		return s.wait()
	}
	if s.containerID.Parent != nil {
		return s.wait()
	}
	return 0, nil
}

func (s *taskIO) prepareTerminal(enabled bool) (func(), <-chan [2]int, error) {
	resize := make(chan [2]int, 4)
	if !enabled {
		return func() {}, resize, nil
	}
	file, ok := s.stdin.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return nil, nil, fmt.Errorf("Must be running in a tty to pass the '--tty flag'")
	}
	state, err := term.MakeRaw(int(file.Fd()))
	if err != nil {
		return nil, nil, err
	}
	resize <- [2]int{0, 0}
	if columns, rows, err := term.GetSize(int(file.Fd())); err == nil {
		resize <- [2]int{rows, columns}
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-signals:
				if columns, rows, err := term.GetSize(int(file.Fd())); err == nil {
					select {
					case resize <- [2]int{rows, columns}:
					default:
					}
				}
			case <-done:
				return
			}
		}
	}()
	return func() { close(done); signal.Stop(signals); term.Restore(int(file.Fd()), state) }, resize, nil
}

func (s *taskIO) initialInput() []byte {
	return encodeInput(map[string]any{"type": "ATTACH_CONTAINER_INPUT", "attach_container_input": map[string]any{"type": "CONTAINER_ID", "container_id": s.containerID}})
}
func encodeInput(message map[string]any) []byte {
	data, _ := json.Marshal(message)
	return recordio.Encode(data)
}
func dataInput(data []byte) []byte {
	return encodeInput(map[string]any{"type": "ATTACH_CONTAINER_INPUT", "attach_container_input": map[string]any{"type": "PROCESS_IO", "process_io": map[string]any{"type": "DATA", "data": map[string]any{"type": "STDIN", "data": base64.StdEncoding.EncodeToString(data)}}}})
}
func heartbeatInput() []byte {
	return encodeInput(map[string]any{"type": "ATTACH_CONTAINER_INPUT", "attach_container_input": map[string]any{"type": "PROCESS_IO", "process_io": map[string]any{"type": "CONTROL", "control": map[string]any{"type": "HEARTBEAT", "heartbeat": map[string]any{"interval": map[string]any{"nanoseconds": int64(30 * time.Second)}}}}}})
}
func resizeInput(size [2]int) []byte {
	return encodeInput(map[string]any{"type": "ATTACH_CONTAINER_INPUT", "attach_container_input": map[string]any{"type": "PROCESS_IO", "process_io": map[string]any{"type": "CONTROL", "control": map[string]any{"type": "TTY_INFO", "tty_info": map[string]any{"window_size": map[string]any{"rows": size[0], "columns": size[1]}}}}}})
}

func (s *taskIO) inputRequest(body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, s.agentURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/recordio")
	req.Header.Set("Message-Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Connection", "close")
	if user, secret, ok := s.plugin.config.AgentAuthentication(); ok {
		req.SetBasicAuth(user, secret)
	}
	return s.plugin.stream.Do(req)
}
func (s *taskIO) streamInput(resize <-chan [2]int, detectExit bool) error {
	handshake, err := s.inputRequest(bytes.NewReader(s.initialInput()))
	if err != nil {
		return err
	}
	io.Copy(io.Discard, handshake.Body)
	handshake.Body.Close()
	if handshake.StatusCode != http.StatusOK && handshake.StatusCode != http.StatusInternalServerError {
		return fmt.Errorf("%s", handshake.Status)
	}
	reader, writer := io.Pipe()
	writeDone := make(chan error, 1)
	go func() {
		defer writer.Close()
		if _, err := writer.Write(s.initialInput()); err != nil {
			writeDone <- err
			return
		}
		chunks := make(chan []byte)
		readErrors := make(chan error, 1)
		go func() {
			buffer := make([]byte, 1024)
			for {
				n, err := s.stdin.Read(buffer)
				if n > 0 {
					chunks <- append([]byte(nil), buffer[:n]...)
				}
				if err != nil {
					readErrors <- err
					return
				}
			}
		}()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		pending := []byte{}
		for {
			select {
			case size := <-resize:
				if _, err := writer.Write(resizeInput(size)); err != nil {
					writeDone <- err
					return
				}
			case chunk := <-chunks:
				combined := append(pending, chunk...)
				pending = nil
				if detectExit && bytes.Contains(combined, []byte{0x10, 0x11}) {
					writer.Write(dataInput(nil))
					writeDone <- nil
					return
				}
				if len(combined) > 0 {
					if _, err := writer.Write(dataInput(combined)); err != nil {
						writeDone <- err
						return
					}
				}
			case err := <-readErrors:
				if err == io.EOF {
					_, err = writer.Write(dataInput(nil))
					writeDone <- err
					return
				}
				writeDone <- err
				return
			case <-ticker.C:
				if _, err := writer.Write(heartbeatInput()); err != nil {
					writeDone <- err
					return
				}
			}
		}
	}()
	response, err := s.inputRequest(reader)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if err := <-writeDone; err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s", response.Status)
	}
	return nil
}
