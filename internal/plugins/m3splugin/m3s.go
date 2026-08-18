package m3splugin

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
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

type M3SPlugin struct {
	client      FrameworkClient
	credentials CredentialProvider
	httpClient  *http.Client
}

func New(client FrameworkClient, credentials CredentialProvider, httpClient *http.Client) *M3SPlugin {
	return &M3SPlugin{client: client, credentials: credentials, httpClient: httpClient}
}

func (*M3SPlugin) Name() string        { return "m3s" }
func (*M3SPlugin) Description() string { return "Interacts with the Kubernetes Framework M3s" }

func (p *M3SPlugin) Run(args []string, _ io.Reader, stdout, _ io.Writer) (int, error) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, err := io.WriteString(stdout, m3sHelp)
		return 0, err
	}
	if args[0] == "--version" {
		_, err := io.WriteString(stdout, "0.1.0\n")
		return 0, err
	}
	switch args[0] {
	case "list":
		if len(args) > 2 || (len(args) == 2 && args[1] != "--all" && args[1] != "-a") {
			return 1, errors.New("usage: mesos m3s list [--all]")
		}
		frameworks, err := p.client.Frameworks()
		if err != nil {
			return 1, err
		}
		all := len(args) == 2
		result, err := table.New([]string{"ID", "Active", "WebUI", "Name"})
		if err != nil {
			return 1, err
		}
		for _, framework := range frameworks {
			if !strings.Contains(strings.ToLower(framework.Name), "m3s") || (!all && !framework.Active) {
				continue
			}
			active := "False"
			if framework.Active {
				active = "True"
			}
			if err := result.AddRow([]string{framework.ID, active, framework.WebUIURL, framework.Name}); err != nil {
				return 1, err
			}
		}
		_, err = io.WriteString(stdout, result.String()+"\n")
		return 0, err
	case "kubeconfig":
		if len(args) != 2 {
			return 1, errors.New("missing <framework-name>")
		}
		body, err := p.request(args[1], http.MethodGet, "/api/m3s/v0/server/config")
		if err != nil {
			return 1, err
		}
		_, err = io.WriteString(stdout, string(body)+"\n")
		return 0, err
	case "version":
		if len(args) != 2 {
			return 1, errors.New("missing <framework-name>")
		}
		body, err := p.request(args[1], http.MethodGet, "/api/m3s/v0/server/version")
		if err != nil {
			return 1, err
		}
		_, err = io.WriteString(stdout, string(body)+"\n")
		return 0, err
	case "status":
		if len(args) < 3 {
			return 1, errors.New("usage: mesos m3s status <framework-name> [--m3s | --kubernetes]")
		}
		wantM3S, wantKubernetes := false, false
		for _, flag := range args[2:] {
			switch flag {
			case "--m3s", "-m":
				wantM3S = true
			case "--kubernetes", "-k":
				wantKubernetes = true
			default:
				return 1, errors.New("unknown status flag '" + flag + "'")
			}
		}
		for _, selected := range []struct {
			enabled  bool
			endpoint string
		}{{wantM3S, "/api/m3s/v0/status/m3s"}, {wantKubernetes, "/api/m3s/v0/status/k8s"}} {
			if !selected.enabled {
				continue
			}
			body, err := p.request(args[1], http.MethodGet, selected.endpoint)
			if err != nil {
				return 1, err
			}
			if _, err := io.WriteString(stdout, string(body)+"\n"); err != nil {
				return 1, err
			}
		}
		return 0, nil
	case "scale":
		if len(args) != 4 {
			return 1, errors.New("usage: mesos m3s scale <framework-name> <count> [--agent | --etcd]")
		}
		service := ""
		switch args[3] {
		case "--agent", "-a":
			service = "agent"
		case "--etcd", "-e":
			service = "datastore"
		default:
			return 1, errors.New("scale requires either --agent or --etcd")
		}
		body, err := p.request(args[1], http.MethodGet, "/api/m3s/v0/"+service+"/scale/"+args[2])
		if err != nil {
			return 1, err
		}
		_, err = io.WriteString(stdout, string(body)+"\n")
		return 0, err
	case "cluster":
		if len(args) != 3 {
			return 1, errors.New("usage: mesos m3s cluster <framework-name> <stop|start|restart>")
		}
		endpoint := ""
		switch args[2] {
		case "stop":
			endpoint = "/api/m3s/v0/cluster/shutdown"
		case "start":
			endpoint = "/api/m3s/v0/cluster/start"
		case "restart":
			endpoint = "/api/m3s/v0/cluster/restart"
		default:
			return 1, errors.New("unknown cluster operation '" + args[2] + "'")
		}
		body, err := p.request(args[1], http.MethodPut, endpoint)
		if err != nil {
			return 1, err
		}
		_, err = io.WriteString(stdout, string(body)+"\n")
		return 0, err
	default:
		return 1, errors.New("unknown m3s command '" + args[0] + "'")
	}
}

func (p *M3SPlugin) resolve(reference string) (mesos.Framework, error) {
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

func (p *M3SPlugin) request(reference, method, endpoint string) ([]byte, error) {
	framework, err := p.resolve(reference)
	if err != nil {
		logrus.WithError(err).Error("unable to resolve M3S framework")
		return nil, err
	}
	credentials, err := p.credentials.FrameworkCredentials("m3s", reference)
	if err != nil {
		logrus.WithError(err).Error("unable to load M3S credentials")
		return nil, err
	}
	request, err := http.NewRequest(method, strings.TrimRight(framework.WebUIURL, "/")+endpoint, bytes.NewReader(nil))
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
		logrus.WithError(err).Error("M3S API request failed")
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

const m3sHelp = `Interacts with the Kubernetes Framework M3s

Usage:
  mesos m3s (-h | --help)
  mesos m3s --version
  mesos m3s <command> [<args>...]

Commands:
  cluster     Control the Kubernetes cluster.
  kubeconfig  Get kubernetes configuration file.
  list        Show list of running M3s frameworks.
  scale       Scale agents or etcd/datastore.
  status      Get M3s and Kubernetes status.
  version     Get the Kubernetes version.
`
