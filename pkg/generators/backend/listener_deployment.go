package backend

import (
	"fmt"

	"github.com/3scale/saas-operator/pkg/basereconciler"
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
func (gen *ListenerGenerator) Deployment() basereconciler.GeneratorFunction {

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
									if *gen.ListenerSpec.Config.RedisAsync {
										args = []string{"bin/3scale_backend", "-s", "falcon", "start"}
									} else {
										args = []string{"bin/3scale_backend", "start"}
									}
									args = append(args, "-e", "production", "-p", "3000", "-x", "/dev/stdout")
									return
								}(),
								Ports: pod.ContainerPorts(
									pod.ContainerPortTCP("http", 3000),
									pod.ContainerPortTCP("metrics", 9394),
								),
								Env:                      pod.BuildEnvironment(gen.Options),
								Resources:                corev1.ResourceRequirements(*gen.ListenerSpec.Resources),
								ImagePullPolicy:          *gen.Image.PullPolicy,
								LivenessProbe:            pod.TCPProbe(intstr.FromString("http"), *gen.ListenerSpec.LivenessProbe),
								ReadinessProbe:           pod.HTTPProbe("/status", intstr.FromString("http"), corev1.URISchemeHTTP, *gen.ListenerSpec.ReadinessProbe),
								TerminationMessagePath:   corev1.TerminationMessagePathDefault,
								TerminationMessagePolicy: corev1.TerminationMessageReadFile,
							},
						},
						Affinity:    pod.Affinity(gen.Selector().MatchLabels, gen.ListenerSpec.NodeAffinity),
						Tolerations: gen.ListenerSpec.Tolerations,
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
