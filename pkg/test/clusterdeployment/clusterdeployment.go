package clusterdeployment

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.ClusterDeployment)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.ClusterDeployment {
	retval := &hivev1.ClusterDeployment{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hivev1.ClusterDeployment

	Options(opts ...Option) Builder

	GenericOptions(opts ...generic.Option) Builder
}

func BasicBuilder() Builder {
	return &builder{}
}

func FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder {
	b := &builder{}
	return b.GenericOptions(
		generic.WithTypeMeta(typer),
		generic.WithResourceVersion("1"),
		generic.WithNamespace(namespace),
		generic.WithName(name),
	)
}

type builder struct {
	options []Option
}

func (b *builder) Build(opts ...Option) *hivev1.ClusterDeployment {
	return Build(append(b.options, opts...)...)
}

func (b *builder) Options(opts ...Option) Builder {
	return &builder{
		options: append(b.options, opts...),
	}
}

func (b *builder) GenericOptions(opts ...generic.Option) Builder {
	options := make([]Option, len(opts))
	for i, o := range opts {
		options[i] = Generic(o)
	}
	return b.Options(options...)
}

// Generic allows common functions applicable to all objects to be used as Options to Build
func Generic(opt generic.Option) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		opt(clusterDeployment)
	}
}

// WithName sets the object.Name field when building an object with Build.
func WithName(name string) Option {
	return Generic(generic.WithName(name))
}

// WithNamespace sets the object.Namespace field when building an object with Build.
func WithNamespace(namespace string) Option {
	return Generic(generic.WithNamespace(namespace))
}

// WithLabel sets the specified label on the supplied object.
func WithLabel(key, value string) Option {
	return Generic(generic.WithLabel(key, value))
}

// WithAnnotation sets the specified annotation on the supplied object.
func WithAnnotation(key, value string) Option {
	return Generic(generic.WithAnnotation(key, value))
}

// WithCondition adds the specified condition to the ClusterDeployment
func WithCondition(cond hivev1.ClusterDeploymentCondition) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		for i, c := range clusterDeployment.Status.Conditions {
			if c.Type == cond.Type {
				clusterDeployment.Status.Conditions[i] = cond
				return
			}
		}
		clusterDeployment.Status.Conditions = append(clusterDeployment.Status.Conditions, cond)
	}
}

func WithUnclaimedClusterPoolReference(namespace, poolName string) Option {
	return WithClusterPoolReference(namespace, poolName, "")
}

func WithClusterPoolReference(namespace, poolName, claimName string) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.ClusterPoolRef = &hivev1.ClusterPoolReference{
			Namespace: namespace,
			PoolName:  poolName,
			ClaimName: claimName,
		}
	}
}

func Installed() Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Installed = true
	}
}

func InstalledTimestamp(instTime time.Time) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Installed = true
		clusterDeployment.Status.InstalledTimestamp = &metav1.Time{Time: instTime}
	}
}

func InstallRestarts(restarts int) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Status.InstallRestarts = restarts
	}
}

func WithClusterVersion(version string) Option {
	return Generic(generic.WithLabel(constants.VersionMajorMinorPatchLabel, version))
}

func WithPowerState(powerState hivev1.ClusterPowerState) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.PowerState = powerState
	}
}

func WithHibernateAfter(dur time.Duration) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.HibernateAfter = &metav1.Duration{Duration: dur}
	}
}

// WithAWSPlatform sets the specified aws platform on the supplied object.
func WithAWSPlatform(platform *hivev1aws.Platform) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Platform.AWS = platform
	}
}
