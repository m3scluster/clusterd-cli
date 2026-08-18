package composeplugin

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"mesos-cli/internal/config"
	"mesos-cli/internal/mesos"
	"mesos-cli/internal/table"
)

type FrameworkClient interface {
	Frameworks() ([]mesos.Framework, error)
}

type CredentialProvider interface {
	FrameworkCredentials(pluginName, frameworkName string) (config.FrameworkCredentials, error)
}

type ComposePlugin struct {
	client      FrameworkClient
	credentials CredentialProvider
	httpClient  *http.Client
}

func New(client FrameworkClient, credentials CredentialProvider, httpClient *http.Client) *ComposePlugin {
	return &ComposePlugin{client: client, credentials: credentials, httpClient: httpClient}
}

func (*ComposePlugin) Name() string        { return "compose" }
func (*ComposePlugin) Description() string { return "Interacts with the Mesos-Compose Framework" }

func (p *ComposePlugin) Run(args []string, _ io.Reader, stdout, _ io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, err := io.WriteString(stdout, composeHelp)
		return 0, err
	}
	if args[0] == "--version" {
		_, err := io.WriteString(stdout, "0.1.0\n")
		return 0, err
	}
	switch args[0] {
	case "version":
		if len(args) != 2 {
			return 1, errors.New("missing <framework-name>")
		}
		body, err := p.request(args[1], http.MethodGet, "/api/compose/versions", nil)
		if err != nil {
			return 1, err
		}
		_, err = io.WriteString(stdout, string(body)+"\n")
		return 0, err
	case "list":
		if len(args) != 2 {
			return 1, errors.New("missing <framework-name>")
		}
		body, err := p.request(args[1], http.MethodGet, "/api/compose/v0/tasks", nil)
		if err != nil {
			return 1, err
		}
		var tasks []struct {
			TaskID   string `json:"TaskID"`
			TaskName string `json:"task_name"`
			State    string `json:"State"`
			Agent    struct {
				Hostname string `json:"hostname"`
			} `json:"MesosAgent"`
		}
		if err := json.Unmarshal(body, &tasks); err != nil {
			return 1, err
		}
		if len(tasks) == 0 {
			_, err = io.WriteString(stdout, "There are no tasks running in the cluster.\n")
			return 0, err
		}
		result, err := table.New([]string{"ID", "NAME", "STATE", "AGENT"})
		if err != nil {
			return 1, err
		}
		for _, task := range tasks {
			if err := result.AddRow([]string{task.TaskID, task.TaskName, task.State, task.Agent.Hostname}); err != nil {
				return 1, err
			}
		}
		_, err = io.WriteString(stdout, result.String()+"\n")
		return 0, err
	case "info":
		if len(args) != 2 {
			return 1, errors.New("missing <framework-name>")
		}
		framework, err := p.resolve(args[1])
		if err != nil {
			return 1, err
		}
		body, err := json.MarshalIndent(framework, "", "  ")
		if err != nil {
			return 1, err
		}
		_, err = io.WriteString(stdout, string(body)+"\n")
		return 0, err
	case "launch":
		if len(args) != 4 {
			return 1, errors.New("usage: mesos compose launch <framework-name> <project> <compose-file>")
		}
		body, err := p.fileRequest(args[1], args[2], args[3], http.MethodPut)
		if err != nil {
			return 1, err
		}
		err = writePrettyJSON(stdout, body)
		return 0, err
	case "update":
		if len(args) != 4 {
			return 1, errors.New("usage: mesos compose update <framework-name> <project> <compose-file>")
		}
		body, err := p.fileRequest(args[1], args[2], args[3], "UPDATE")
		if err != nil {
			return 1, err
		}
		err = writePrettyJSON(stdout, body)
		return 0, err
	case "kill":
		if len(args) != 3 {
			return 1, errors.New("usage: mesos compose kill <framework-name> <task>")
		}
		endpoint := "/api/compose/v0/tasks/" + args[2]
		if parts := strings.Split(args[2], ":"); len(parts) >= 3 {
			endpoint = "/api/compose/v0/" + parts[1] + "/" + parts[2]
		}
		body, err := p.request(args[1], http.MethodDelete, endpoint, nil)
		if err != nil {
			return 1, err
		}
		_, err = io.WriteString(stdout, string(body)+"\n")
		return 0, err
	case "restart":
		if len(args) != 3 {
			return 1, errors.New("usage: mesos compose restart <framework-name> <task>")
		}
		parts := strings.Split(args[2], ":")
		if len(parts) < 3 {
			return 1, errors.New("restart requires a service name in '<prefix>:<project>:<service>' format")
		}
		endpoint := "/api/compose/v0/" + parts[1] + "/" + parts[2] + "/restart"
		body, err := p.request(args[1], http.MethodPut, endpoint, nil)
		if err != nil {
			return 1, err
		}
		_, err = io.WriteString(stdout, string(body)+"\n")
		return 0, err
	case "framework":
		if len(args) != 3 {
			return 1, errors.New("usage: mesos compose framework <framework-name> <reregister|suppress>")
		}
		endpoint := ""
		switch args[2] {
		case "reregister":
			endpoint = "/api/compose/v0/framework/reregister"
		case "suppress":
			endpoint = "/api/compose/v0/framework/suppress"
		default:
			return 1, errors.New("unknown framework operation '" + args[2] + "'")
		}
		body, err := p.request(args[1], http.MethodPut, endpoint, nil)
		if err != nil {
			return 1, err
		}
		_, err = io.WriteString(stdout, string(body)+"\n")
		return 0, err
	default:
		return 1, errors.New("unknown compose command '" + args[0] + "'")
	}
}

func (p *ComposePlugin) fileRequest(reference, project, filename, method string) ([]byte, error) {
	composeFile, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return p.request(reference, method, "/api/compose/v0/"+project, composeFile)
}

func writePrettyJSON(stdout io.Writer, body []byte) error {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		_, err = io.WriteString(stdout, string(body)+"\n")
		return err
	}
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, string(formatted)+"\n")
	return err
}

func (p *ComposePlugin) resolve(reference string) (mesos.Framework, error) {
	frameworks, err := p.client.Frameworks()
	if err != nil {
		return mesos.Framework{}, err
	}
	idLike := strings.Count(reference, "-") == 5
	for _, framework := range frameworks {
		if idLike && framework.ID == reference {
			return framework, nil
		}
		if !idLike && framework.Active && strings.EqualFold(framework.Name, reference) {
			return framework, nil
		}
	}
	return mesos.Framework{}, errors.New("Unable to find framework '" + reference + "'")
}

func (p *ComposePlugin) request(reference, method, endpoint string, body []byte) ([]byte, error) {
	framework, err := p.resolve(reference)
	if err != nil {
		logrus.WithError(err).Error("unable to resolve Mesos-Compose framework")
		return nil, err
	}
	credentials, err := p.credentials.FrameworkCredentials("compose", reference)
	if err != nil {
		logrus.WithError(err).Error("unable to load Mesos-Compose credentials")
		return nil, err
	}
	request, err := http.NewRequest(method, strings.TrimRight(framework.WebUIURL, "/")+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if credentials.Principal != "" && credentials.Secret != "" {
		request.SetBasicAuth(credentials.Principal, credentials.Secret)
	}
	client := p.httpClient
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: !credentials.SSLVerify}
		client = &http.Client{Transport: transport, Timeout: 5 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		logrus.WithError(err).Error("Mesos-Compose API request failed")
		return nil, errors.New("Unable to open framework URL: " + err.Error())
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("Framework API returned " + response.Status + ": " + string(data))
	}
	return data, nil
}

const composeHelp = `Interacts with the Mesos-Compose Framework

Usage:
  mesos compose (-h | --help)
  mesos compose --version
  mesos compose <command> [<args>...]

Commands:
  framework  Framework Commands.
  info       Get information about the running Mesos compose framework.
  kill       Kill a single task (ID) or a whole service (Task Name)
  launch     Launch Mesos workload from compose file
  list       Show all running tasks.
  restart    Restart a single task (ID) or a whole service (Task Name)
  update     Update service from compose file
  version    Get the version number of Mesos compose
`
