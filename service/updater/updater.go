package updater

import (
	"strings"

	microerror "github.com/giantswarm/microkit/error"
	micrologger "github.com/giantswarm/microkit/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"

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

func (p *Updater) Update(namespace, service string, podInfos []provider.PodInfo) error {
	for _, pi := range podInfos {
		s, err := p.kubernetesClient.Services(namespace).Get(service, metav1.GetOptions{})
		if err != nil {
			return microerror.MaskAny(err)
		}

		endpoint := &apiv1.Endpoints{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: service,
			},
			Subsets: []apiv1.EndpointSubset{
				{
					Addresses: []apiv1.EndpointAddress{
						{
							IP: pi.IP.String(),
						},
					},
					Ports: serviceToPorts(s),
				},
			},
		}

		_, err = p.kubernetesClient.Endpoints(namespace).Create(endpoint)
		if err != nil {
			// TODO super dirty hack to prevent workers from crashing the whole guest
			// cluster.
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return microerror.MaskAny(err)
		}
	}

	return nil
}

func podInfoByName(podInfos []provider.PodInfo, name string) (provider.PodInfo, error) {
	for _, pi := range podInfos {
		if pi.Name == name {
			return pi, nil
		}
	}

	return provider.PodInfo{}, microerror.MaskAnyf(executionFailedError, "pod info for name '%s' not found", name)
}

func serviceToPorts(s *apiv1.Service) []apiv1.EndpointPort {
	var ports []apiv1.EndpointPort

	for _, p := range s.Spec.Ports {
		port := apiv1.EndpointPort{
			Name: p.Name,
			Port: p.Port,
		}

		ports = append(ports, port)
	}

	return ports
}
