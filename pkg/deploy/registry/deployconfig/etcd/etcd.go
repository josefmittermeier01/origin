package etcd

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
	kapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	extvalidation "k8s.io/kubernetes/pkg/apis/extensions/validation"

	"github.com/openshift/origin/pkg/deploy/api"
	"github.com/openshift/origin/pkg/deploy/registry/deployconfig"
	"github.com/openshift/origin/pkg/util/restoptions"
)

// REST contains the REST storage for DeploymentConfig objects.
type REST struct {
	*registry.Store
}

// NewREST returns a deploymentConfigREST containing the REST storage for DeploymentConfig objects,
// a statusREST containing the REST storage for changing the status of a DeploymentConfig,
// and a scaleREST containing the REST storage for the Scale subresources of DeploymentConfigs.
func NewREST(optsGetter restoptions.Getter) (*REST, *StatusREST, *ScaleREST, error) {
	store := &registry.Store{
		Copier:            kapi.Scheme,
		NewFunc:           func() runtime.Object { return &api.DeploymentConfig{} },
		NewListFunc:       func() runtime.Object { return &api.DeploymentConfigList{} },
		PredicateFunc:     deployconfig.Matcher,
		QualifiedResource: api.Resource("deploymentconfigs"),

		CreateStrategy: deployconfig.Strategy,
		UpdateStrategy: deployconfig.Strategy,
		DeleteStrategy: deployconfig.Strategy,
	}

	options := &generic.StoreOptions{RESTOptions: optsGetter, AttrFunc: deployconfig.GetAttrs}
	if err := store.CompleteWithOptions(options); err != nil {
		return nil, nil, nil, err
	}

	deploymentConfigREST := &REST{store}

	statusStore := *store
	statusStore.UpdateStrategy = deployconfig.StatusStrategy

	statusREST := &StatusREST{store: &statusStore}
	scaleREST := &ScaleREST{registry: deployconfig.NewRegistry(deploymentConfigREST)}

	return deploymentConfigREST, statusREST, scaleREST, nil
}

// ScaleREST contains the REST storage for the Scale subresource of DeploymentConfigs.
type ScaleREST struct {
	registry deployconfig.Registry
}

// ScaleREST implements Patcher
var _ = rest.Patcher(&ScaleREST{})

// New creates a new Scale object
func (r *ScaleREST) New() runtime.Object {
	return &extensions.Scale{}
}

// Get retrieves (computes) the Scale subresource for the given DeploymentConfig name.
func (r *ScaleREST) Get(ctx apirequest.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	deploymentConfig, err := r.registry.GetDeploymentConfig(ctx, name, options)
	if err != nil {
		return nil, err
	}

	return api.ScaleFromConfig(deploymentConfig), nil
}

// Update scales the DeploymentConfig for the given Scale subresource, returning the updated Scale.
func (r *ScaleREST) Update(ctx apirequest.Context, name string, objInfo rest.UpdatedObjectInfo) (runtime.Object, bool, error) {
	deploymentConfig, err := r.registry.GetDeploymentConfig(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, errors.NewNotFound(extensions.Resource("scale"), name)
	}

	old := api.ScaleFromConfig(deploymentConfig)
	obj, err := objInfo.UpdatedObject(ctx, old)
	if err != nil {
		return nil, false, err
	}

	scale, ok := obj.(*extensions.Scale)
	if !ok {
		return nil, false, errors.NewBadRequest(fmt.Sprintf("wrong object passed to Scale update: %v", obj))
	}

	if errs := extvalidation.ValidateScale(scale); len(errs) > 0 {
		return nil, false, errors.NewInvalid(extensions.Kind("Scale"), scale.Name, errs)
	}

	deploymentConfig.Spec.Replicas = scale.Spec.Replicas
	if err := r.registry.UpdateDeploymentConfig(ctx, deploymentConfig); err != nil {
		return nil, false, err
	}

	return scale, false, nil
}

// StatusREST implements the REST endpoint for changing the status of a DeploymentConfig.
type StatusREST struct {
	store *registry.Store
}

// StatusREST implements Patcher
var _ = rest.Patcher(&StatusREST{})

func (r *StatusREST) New() runtime.Object {
	return &api.DeploymentConfig{}
}

// Get retrieves the object from the storage. It is required to support Patch.
func (r *StatusREST) Get(ctx apirequest.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return r.store.Get(ctx, name, options)
}

// Update alters the status subset of an deploymentConfig.
func (r *StatusREST) Update(ctx apirequest.Context, name string, objInfo rest.UpdatedObjectInfo) (runtime.Object, bool, error) {
	return r.store.Update(ctx, name, objInfo)
}
