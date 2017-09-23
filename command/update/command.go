// Package update implements the update command for the command line tool.
package update

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cenk/backoff"
	microerror "github.com/giantswarm/microkit/error"
	micrologger "github.com/giantswarm/microkit/logger"
	"github.com/giantswarm/micrologger/microloggertest"
	"github.com/giantswarm/operatorkit/client/k8s"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"

	"github.com/giantswarm/k8s-endpoint-updater/command/update/flag"
	"github.com/giantswarm/k8s-endpoint-updater/service/provider"
	"github.com/giantswarm/k8s-endpoint-updater/service/provider/bridge"
	"github.com/giantswarm/k8s-endpoint-updater/service/updater"
)

var (
	f = &flag.Flag{}
)

// Config represents the configuration used to create a new update command.
type Config struct {
	// Dependencies.
	Logger micrologger.Logger
}

// DefaultConfig provides a default configuration to create a new update
// command by best effort.
func DefaultConfig() Config {
	return Config{
		// Dependencies.
		Logger: nil,
	}
}

// New creates a new configured update command.
func New(config Config) (*Command, error) {
	// Dependencies.
	if config.Logger == nil {
		return nil, microerror.MaskAnyf(invalidConfigError, "logger must not be empty")
	}

	newCommand := &Command{
		// Dependencies.
		logger: config.Logger,

		// Internals.
		cobraCommand: nil,
	}

	newCommand.cobraCommand = &cobra.Command{
		Use:   "update",
		Short: "Update Kubernetes endpoints based on given configuration.",
		Long:  "Update Kubernetes endpoints based on given configuration.",
		Run:   newCommand.Execute,
	}

	newCommand.CobraCommand().PersistentFlags().StringVar(&f.Kubernetes.Address, "service.kubernetes.address", "http://127.0.0.1:6443", "Address used to connect to Kubernetes. When empty in-cluster config is created.")
	newCommand.CobraCommand().PersistentFlags().StringVar(&f.Kubernetes.Cluster.Namespace, "service.kubernetes.cluster.namespace", "default", "Namespace of the guest cluster which endpoints should be updated.")
	newCommand.CobraCommand().PersistentFlags().StringVar(&f.Kubernetes.Cluster.Service, "service.kubernetes.cluster.service", "", "Name of the service which endpoints should be updated.")
	newCommand.CobraCommand().PersistentFlags().BoolVar(&f.Kubernetes.InCluster, "service.kubernetes.inCluster", false, "Whether to use the in-cluster config to authenticate with Kubernetes.")
	newCommand.CobraCommand().PersistentFlags().StringVar(&f.Kubernetes.TLS.CaFile, "service.kubernetes.tls.caFile", "", "Certificate authority file path to use to authenticate with Kubernetes.")
	newCommand.CobraCommand().PersistentFlags().StringVar(&f.Kubernetes.TLS.CrtFile, "service.kubernetes.tls.crtFile", "", "Certificate file path to use to authenticate with Kubernetes.")
	newCommand.CobraCommand().PersistentFlags().StringVar(&f.Kubernetes.TLS.KeyFile, "service.kubernetes.tls.keyFile", "", "Key file path to use to authenticate with Kubernetes.")

	newCommand.cobraCommand.PersistentFlags().StringVar(&f.Provider.Bridge.Name, "provider.bridge.name", "", "Bridge name of the guest cluster VM on the host network.")
	newCommand.cobraCommand.PersistentFlags().StringVar(&f.Provider.Env.Prefix, "provider.env.prefix", "K8S_ENDPOINT_UPDATER_POD_", "Prefix of environment variables providing pod names.")
	newCommand.cobraCommand.PersistentFlags().StringVar(&f.Provider.Etcd.Address, "provider.etcd.address", "", "Address used to connect to etcd.")
	newCommand.cobraCommand.PersistentFlags().StringVar(&f.Provider.Etcd.Kind, "provider.etcd.kind", "etcdv2", "Etcd storage client version to use.")
	newCommand.cobraCommand.PersistentFlags().StringVar(&f.Provider.Etcd.Prefix, "provider.etcd.prefix", "", "Prefix of etcd paths providing pod names.")
	newCommand.cobraCommand.PersistentFlags().StringVar(&f.Provider.Kind, "provider.kind", "env", "Provider used to lookup pod IPs.")

	return newCommand, nil
}

type Command struct {
	// Dependencies.
	logger micrologger.Logger

	// Internals.
	cobraCommand *cobra.Command
}

func (c *Command) CobraCommand() *cobra.Command {
	return c.cobraCommand
}

func (c *Command) Execute(cmd *cobra.Command, args []string) {
	c.logger.Log("info", "start updating Kubernetes endpoint")

	err := f.Validate()
	if err != nil {
		c.logger.Log("error", fmt.Sprintf("%#v", microerror.MaskAny(err)))
		os.Exit(1)
	}

	err = c.execute()
	if err != nil {
		c.logger.Log("error", fmt.Sprintf("%#v", microerror.MaskAny(err)))
		os.Exit(1)
	}

	c.logger.Log("info", "finished updating Kubernetes endpoint")
}

func (c *Command) execute() error {
	var err error

	var k8sClient kubernetes.Interface
	{
		k8sConfig := k8s.DefaultConfig()

		k8sConfig.Address = f.Kubernetes.Address
		k8sConfig.Logger = microloggertest.New()
		k8sConfig.InCluster = f.Kubernetes.InCluster
		k8sConfig.TLS.CAFile = f.Kubernetes.TLS.CaFile
		k8sConfig.TLS.CrtFile = f.Kubernetes.TLS.CrtFile
		k8sConfig.TLS.KeyFile = f.Kubernetes.TLS.KeyFile

		k8sClient, err = k8s.NewClient(k8sConfig)
		if err != nil {
			return microerror.MaskAny(err)
		}
	}

	// At first we have to sort out which provider to use. This is based on the
	// flags given to the updater.
	var newProvider provider.Provider
	{
		bridgeConfig := bridge.DefaultConfig()

		bridgeConfig.Logger = c.logger

		bridgeConfig.BridgeName = f.Provider.Bridge.Name

		newProvider, err = bridge.New(bridgeConfig)
		if err != nil {
			return microerror.MaskAny(err)
		}
	}

	// We also need to create the updater which is able to update Kubernetes
	// endpoints.
	var newUpdater *updater.Updater
	{
		updaterConfig := updater.DefaultConfig()

		updaterConfig.K8sClient = k8sClient
		updaterConfig.Logger = c.logger

		newUpdater, err = updater.New(updaterConfig)
		if err != nil {
			return microerror.MaskAny(err)
		}
	}

	// Once we know which provider to use we execute it to lookup the pod
	// information we are interested in.
	var podInfos []provider.PodInfo
	{
		action := func() error {
			podInfos, err = newProvider.Lookup()
			if err != nil {
				return microerror.MaskAny(err)
			}

			return nil
		}

		notifier := func(err error, d time.Duration) {
			fmt.Printf("%#v\n", err)
		}

		err := backoff.RetryNotify(action, backoff.NewExponentialBackOff(), notifier)
		if err != nil {
			return microerror.MaskAny(err)
		}

		for _, pi := range podInfos {
			c.logger.Log("debug", fmt.Sprintf("found pod info of service '%s'", f.Kubernetes.Cluster.Service), "ip", pi.IP.String())
		}
	}

	// Use the updater to actually add the endpoints identified by the provided
	// flags.
	{
		action := func() error {
			err := newUpdater.Create(f.Kubernetes.Cluster.Namespace, f.Kubernetes.Cluster.Service, podInfos)
			if err != nil {
				return microerror.MaskAny(err)
			}

			return nil
		}

		err := backoff.Retry(action, backoff.NewExponentialBackOff())
		if err != nil {
			return microerror.MaskAny(err)
		}

		c.logger.Log("debug", fmt.Sprintf("added IPs to endpoint of service '%s'", f.Kubernetes.Cluster.Service))
	}

	// Listen to OS signals issued by the Kubernetes scheduler.
	listener := make(chan os.Signal, 2)
	signal.Notify(listener, syscall.SIGTERM, syscall.SIGKILL)

	fmt.Printf("bridge name : %#v\n", f.Provider.Bridge.Name)
	fmt.Printf("waiting for termination signals\n")

	s1 := <-listener

	fmt.Printf("received termination signal: %#v (%s)\n", s1, s1)

	// Use the updater to actually delete the endpoints identified by the provided
	// flags.
	go func() {
		action := func() error {
			err := newUpdater.Delete(f.Kubernetes.Cluster.Namespace, f.Kubernetes.Cluster.Service, podInfos)
			if err != nil {
				return microerror.MaskAny(err)
			}

			return nil
		}

		err := backoff.Retry(action, backoff.NewExponentialBackOff())
		if err != nil {
			c.logger.Log("error", fmt.Sprintf("%#v", microerror.MaskAny(err)))
			os.Exit(1)
		}

		c.logger.Log("debug", fmt.Sprintf("removed IPs from endpoint of service '%s'", f.Kubernetes.Cluster.Service))

		os.Exit(0)
	}()

	<-listener

	os.Exit(0)

	return nil
}
