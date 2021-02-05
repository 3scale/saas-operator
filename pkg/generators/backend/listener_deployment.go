package backend

import (
	"fmt"
	"strconv"

	saasv1alpha1 "github.com/3scale/saas-operator/api/v1alpha1"
	"github.com/3scale/saas-operator/pkg/basereconciler"
	"github.com/3scale/saas-operator/pkg/generators/backend/config"
	"github.com/3scale/saas-operator/pkg/generators/common_blocks/marin3r"
	"github.com/3scale/saas-operator/pkg/generators/common_blocks/pod"
	"github.com/3scale/saas-operator/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Deployment returns a basereconciler.GeneratorFunction funtion that will return a Deployment
// resource when called
func (gen *ListenerGenerator) Deployment(hashInternalAPI string, hashErrorMonitoring string) basereconciler.GeneratorFunction {

	return func() client.Object {

		dep := &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: appsv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      gen.GetComponent(),
				Namespace: gen.Namespace,
				Labels:    gen.GetLabels(),
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: gen.ListenerSpec.Replicas,
				Selector: gen.Selector(),
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxUnavailable: util.IntStrPtr(intstr.FromInt(0)),
						MaxSurge:       util.IntStrPtr(intstr.FromInt(1)),
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: gen.LabelsWithSelector(),
						Annotations: map[string]string{
							saasv1alpha1.RolloutTriggerAnnotationKeyPrefix + config.InternalAPISecretName:     hashInternalAPI,
							saasv1alpha1.RolloutTriggerAnnotationKeyPrefix + config.ErrorMonitoringSecretName: hashErrorMonitoring,
						},
					},
					Spec: corev1.PodSpec{
						ImagePullSecrets: func() []corev1.LocalObjectReference {
							if gen.Image.PullSecretName != nil {
								return []corev1.LocalObjectReference{{Name: *gen.Image.PullSecretName}}
							}
							return nil
						}(),
						Containers: []corev1.Container{
							{
								Name:  gen.GetComponent(),
								Image: fmt.Sprintf("%s:%s", *gen.Image.Name, *gen.Image.Tag),
								Args: func() (args []string) {
									args = []string{
										"bin/3scale_backend",
										"start",
										"-e production",
										"-p 3000",
										"-x /dev/stdout",
									}
									if *gen.ListenerSpec.Config.RedisAsync {
										args = append(args, "-s falcon")
									}
									return
								}(),
								Ports: pod.ContainerPorts(
									pod.ContainerPortTCP("http", 3000),
									pod.ContainerPortTCP("metrics", 9394),
								),
								Env: pod.GenerateEnvironment(config.ListenerDefault,
									func() map[string]pod.EnvVarValue {
										m := map[string]pod.EnvVarValue{
											config.RackEnv:                     &pod.DirectValue{Value: *gen.Config.RackEnv},
											config.ConfigMasterServiceID:       &pod.DirectValue{Value: fmt.Sprintf("%d", *gen.Config.MasterServiceID)},
											config.ConfigRequestLoggers:        &pod.DirectValue{Value: *gen.ListenerSpec.Config.LogFormat},
											config.ConfigRedisAsync:            &pod.DirectValue{Value: strconv.FormatBool(*gen.ListenerSpec.Config.RedisAsync)},
											config.ListenerWorkers:             &pod.DirectValue{Value: fmt.Sprintf("%d", *gen.ListenerSpec.Config.ListenerWorkers)},
											config.ConfigLegacyReferrerFilters: &pod.DirectValue{Value: strconv.FormatBool(*gen.ListenerSpec.Config.LegacyReferrerFilters)},
											config.ConfigRedisProxy:            &pod.DirectValue{Value: gen.Config.RedisStorageDSN},
											config.ConfigQueuesMasterName:      &pod.DirectValue{Value: gen.Config.RedisQueuesDSN},
											config.ConfigInternalAPIUser:       &pod.SecretRef{SecretName: config.SecretDefinitions.LookupSecretName(config.ConfigInternalAPIUser)},
											config.ConfigInternalAPIPassword:   &pod.SecretRef{SecretName: config.SecretDefinitions.LookupSecretName(config.ConfigInternalAPIPassword)},
										}
										if gen.Config.ErrorMonitoringService != nil && gen.Config.ErrorMonitoringKey != nil {
											m[config.ConfigHoptoadService] = &pod.SecretRef{SecretName: config.SecretDefinitions.LookupSecretName(config.ConfigHoptoadService)}
											m[config.ConfigHoptoadAPIKey] = &pod.SecretRef{SecretName: config.SecretDefinitions.LookupSecretName(config.ConfigHoptoadAPIKey)}
										}
										return m
									}(),
								),
								Resources:                corev1.ResourceRequirements(*gen.ListenerSpec.Resources),
								ImagePullPolicy:          *gen.Image.PullPolicy,
								LivenessProbe:            pod.TCPProbe(intstr.FromString("http"), *gen.ListenerSpec.LivenessProbe),
								ReadinessProbe:           pod.HTTPProbe("/status", intstr.FromString("http"), corev1.URISchemeHTTP, *gen.ListenerSpec.ReadinessProbe),
								TerminationMessagePath:   corev1.TerminationMessagePathDefault,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
						Affinity: pod.Affinity(gen.Selector().MatchLabels),
					},
				},
			},
		}

		if !gen.ListenerSpec.Marin3r.IsDeactivated() {
			dep = marin3r.EnableSidecar(*dep, *gen.ListenerSpec.Marin3r)
		}

		return dep
	}
}