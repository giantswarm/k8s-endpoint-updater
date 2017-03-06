package updater

import (
	"fmt"

	microerror "github.com/giantswarm/microkit/error"
	micrologger "github.com/giantswarm/microkit/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"

	"github.com/giantswarm/k8s-endpoint-updater/service/provider"
)

// Config represents the configuration used to create a new updater.
type Config struct {
	// Dependencies.
	KubernetesClient *kubernetes.Clientset
	Logger           micrologger.Logger
}

// DefaultConfig provides a default configuration to create a new updater
// by best effort.
func DefaultConfig() Config {
	return Config{
		// Dependencies.
		KubernetesClient: nil,
		Logger:           nil,
	}
}

// New creates a new updater.
func New(config Config) (*Updater, error) {
	// Dependencies.
	if config.KubernetesClient == nil {
		return nil, microerror.MaskAnyf(invalidConfigError, "kubernetes client must not be empty")
	}
	if config.Logger == nil {
		return nil, microerror.MaskAnyf(invalidConfigError, "logger must not be empty")
	}

	newUpdater := &Updater{
		// Dependencies.
		kubernetesClient: config.KubernetesClient,
		logger:           config.Logger,
	}

	return newUpdater, nil
}

type Updater struct {
	// Dependencies.
	kubernetesClient *kubernetes.Clientset
	logger           micrologger.Logger
}

func (p *Updater) Update(namespace string, podInfos []provider.PodInfo) error {
	endpoints, err := p.kubernetesClient.Endpoints(namespace).List(v1.ListOptions{})
	if err != nil {
		return microerror.MaskAny(err)
	}

	fmt.Printf("\n")
	for _, e := range endpoints.Items {
		fmt.Printf("before: %#v\n", e)
		for _, s := range e.Subsets {
			for i, _ := range s.Addresses {
				for _, pi := range podInfos {
					if s.Addresses[i].TargetRef.Name != pi.Name {
						continue
					}

					fmt.Printf("%#v\n", s.Addresses[i].IP)
					s.Addresses[i].IP = pi.IP.String()
					fmt.Printf("%#v\n", s.Addresses[i].IP)
				}
			}
		}
		fmt.Printf("\n")
		fmt.Printf("after: %#v\n", e)
		// TODO update the endpoints
	}
	fmt.Printf("\n")

	return nil
}
