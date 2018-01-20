package updater

import (
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"net"
)

const (
	annotationIp = "endpoint.kvm.giantswarm.io/ip"
)

// Config represents the configuration used to create a new updater.
type Config struct {
	// Dependencies.
	K8sClient kubernetes.Interface
	Logger    micrologger.Logger
}

// DefaultConfig provides a default configuration to create a new updater
// by best effort.
func DefaultConfig() Config {
	return Config{
		// Dependencies.
		K8sClient: nil,
		Logger:    nil,
	}
}

// New creates a new updater.
func New(config Config) (*Updater, error) {
	// Dependencies.
	if config.K8sClient == nil {
		return nil, microerror.Maskf(invalidConfigError, "config.K8sClient must not be empty")
	}
	if config.Logger == nil {
		return nil, microerror.Maskf(invalidConfigError, "config.Logger must not be empty")
	}

	newUpdater := &Updater{
		// Dependencies.
		k8sClient: config.K8sClient,
		logger:    config.Logger,
	}

	return newUpdater, nil
}

type Updater struct {
	// Dependencies.
	k8sClient kubernetes.Interface
	logger    micrologger.Logger
}

func (p *Updater) AddAnnotations(namespace, service string, podName string, podIP net.IP) error {
	kvmPod, err := p.k8sClient.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})

	if err != nil {
		p.logger.Log("error", fmt.Sprintf("Fetching kvm pod failed: %#v.", err))
		return microerror.Mask(err)
	}

	patch := fmt.Sprintf("{'metadata':{'annotations': {'endpoint.kvm.giantswarm.io/ip':'%s'}}}", podIP.String())

	_, err = p.k8sClient.CoreV1().Pods(namespace).Patch(kvmPod.Name, types.MergePatchType, []byte(patch))
	if err != nil {
		p.logger.Log("error", fmt.Sprintf("Updating pod annotation failed: %#v.", err))
		return microerror.Mask(err)
	}

	return nil
}
