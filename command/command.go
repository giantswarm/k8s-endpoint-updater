// Package command implements the root command for the command line tool.
package command

import (
	"net/url"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	microerror "github.com/giantswarm/microkit/error"
	micrologger "github.com/giantswarm/microkit/logger"

	"github.com/giantswarm/k8s-endpoint-updater/command/update"
	"github.com/giantswarm/k8s-endpoint-updater/command/version"
	"github.com/giantswarm/k8s-endpoint-updater/service/updater"
)

// Config represents the configuration used to create a new root command.
type Config struct {
	// Dependencies.
	Logger micrologger.Logger

	// Settings.
	KubernetesAddress    string
	KubernetesInCluster  bool
	KubernetesTLSCaFile  string
	KubernetesTLSCrtFile string
	KubernetesTLSKeyFile string

	Description string
	GitCommit   string
	Name        string
	Source      string
}

// DefaultConfig provides a default configuration to create a new root command
// by best effort.
func DefaultConfig() Config {
	return Config{
		// Dependencies.
		Logger: nil,

		// Settings.
		KubernetesAddress:    "",
		KubernetesInCluster:  false,
		KubernetesTLSCaFile:  "",
		KubernetesTLSCrtFile: "",
		KubernetesTLSKeyFile: "",

		Description: "",
		GitCommit:   "",
		Name:        "",
		Source:      "",
	}
}

// New creates a new root command.
func New(config Config) (*Command, error) {
	var err error

	var kubernetesClient *kubernetes.Clientset
	{
		var restConfig *rest.Config

		if config.KubernetesInCluster {
			config.Logger.Log("debug", "creating in-cluster config")
			restConfig, err = rest.InClusterConfig()
			if err != nil {
				return nil, microerror.MaskAny(err)
			}

			if config.KubernetesAddress != "" {
				config.Logger.Log("debug", "using explicit api server")
				restConfig.Host = config.KubernetesAddress
			}
		} else {
			if config.KubernetesAddress == "" {
				return nil, microerror.MaskAnyf(invalidConfigError, "kubernetes address must not be empty")
			}

			config.Logger.Log("debug", "creating out-cluster config")

			// Kubernetes listen URL.
			u, err := url.Parse(config.KubernetesAddress)
			if err != nil {
				return nil, microerror.MaskAny(err)
			}

			restConfig = &rest.Config{
				Host: u.String(),
				TLSClientConfig: rest.TLSClientConfig{
					CAFile:   config.KubernetesTLSCaFile,
					CertFile: config.KubernetesTLSCrtFile,
					KeyFile:  config.KubernetesTLSKeyFile,
				},
			}
		}

		kubernetesClient, err = kubernetes.NewForConfig(restConfig)
		if err != nil {
			return nil, microerror.MaskAny(err)
		}
	}
	var newUpdater *updater.Updater
	{
		updaterConfig := updater.DefaultConfig()
		updaterConfig.KubernetesClient = kubernetesClient
		updaterConfig.Logger = config.Logger
		newUpdater, err = updater.New(updaterConfig)
		if err != nil {
			return nil, microerror.MaskAny(err)
		}
	}

	var updateCommand *update.Command
	{
		updateConfig := update.DefaultConfig()
		updateConfig.Logger = config.Logger
		updateConfig.Updater = newUpdater
		updateCommand, err = update.New(updateConfig)
		if err != nil {
			return nil, microerror.MaskAny(err)
		}
	}

	var versionCommand *version.Command
	{
		versionConfig := version.DefaultConfig()

		versionConfig.Description = config.Description
		versionConfig.GitCommit = config.GitCommit
		versionConfig.Name = config.Name
		versionConfig.Source = config.Source

		versionCommand, err = version.New(versionConfig)
		if err != nil {
			return nil, microerror.MaskAny(err)
		}
	}

	newCommand := &Command{
		// Internals.
		cobraCommand:   nil,
		updateCommand:  updateCommand,
		versionCommand: versionCommand,
	}

	newCommand.cobraCommand = &cobra.Command{
		Use:   config.Name,
		Short: config.Description,
		Long:  config.Description,
		Run:   newCommand.Execute,
	}

	newCommand.cobraCommand.AddCommand(newCommand.updateCommand.CobraCommand())
	newCommand.cobraCommand.AddCommand(newCommand.versionCommand.CobraCommand())

	return newCommand, nil
}

type Command struct {
	// Internals.
	cobraCommand   *cobra.Command
	updateCommand  *update.Command
	versionCommand *version.Command
}

func (c *Command) CobraCommand() *cobra.Command {
	return c.cobraCommand
}

func (c *Command) Execute(cmd *cobra.Command, args []string) {
	cmd.HelpFunc()(cmd, nil)
}

func (c *Command) UpdateCommand() *update.Command {
	return c.updateCommand
}

func (c *Command) VersionCommand() *version.Command {
	return c.versionCommand
}
