/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package randomparagraphapp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	randomv1alpha1 "github.com/richardcase/itsrandomoperator/pkg/apis/random/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	appName  = "random-paragraph-ws"
	appLabel = "app"
	rpaLabel = "rpa-name"

	controllerName = "randomparagraphapp-controller"

	// NOTE: Ideally you'd make this configurable
	podDeletionTimeout  = 3 * time.Minute
	podReadinessTimeout = 5 * time.Minute

	pollInterval = 30 * time.Second
)

// Add creates a new RandomParagraphApp Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	log := logf.Log.WithName(controllerName)
	recorder := mgr.GetRecorder(controllerName)

	return &ReconcileRandomParagraphApp{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		logger:   log,
		recorder: recorder}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to RandomParagraphApp
	err = c.Watch(&source.Kind{Type: &randomv1alpha1.RandomParagraphApp{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to pods (owned by the RandomParagraphApp)
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &randomv1alpha1.RandomParagraphApp{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to services (owned by the RandomParagraphApp)
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &randomv1alpha1.RandomParagraphApp{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileRandomParagraphApp{}

// ReconcileRandomParagraphApp reconciles a RandomParagraphApp object
type ReconcileRandomParagraphApp struct {
	client.Client
	scheme   *runtime.Scheme
	logger   logr.Logger
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a RandomParagraphApp object and makes changes based on the state read
// and what is in the RandomParagraphApp.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=random.acme.com,resources=randomparagraphapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileRandomParagraphApp) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the RandomParagraphApp instance
	instance := &randomv1alpha1.RandomParagraphApp{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	err = r.handleService(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.handlePods(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set the status and update
	instance.Status.SetReadyCondition()
	err = r.Update(context.TODO(), instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileRandomParagraphApp) handlePods(rpa *randomv1alpha1.RandomParagraphApp) error {
	// Get a list of the pods owned by our CRD
	pods := &corev1.PodList{}
	labelSet := map[string]string{
		appLabel: appName,
		rpaLabel: rpa.Name,
	}
	opts := &client.ListOptions{
		Namespace:     rpa.Namespace,
		LabelSelector: labels.SelectorFromSet(labelSet),
	}
	err := r.List(context.TODO(), opts, pods)
	if err != nil {
		return err
	}

	diff := len(pods.Items) - rpa.Spec.Replicas
	if diff == 0 {
		// No action needed
		r.logger.Info("current number of pods equals actual number of pods", "runningpods", rpa.Spec.Replicas)
	}

	if diff < 0 {
		// Not enough replicas - create
		err = r.createPods(rpa, -diff, pods)
		if err != nil {
			return err
		}
		// Add a scale condition
		rpa.Status.SetScaleCondition(len(pods.Items), rpa.Spec.Replicas)
	}
	if diff > 0 {
		// Too many replicas - delete
		err = r.deletePods(rpa, pods, diff)
		if err != nil {
			return err
		}
		// Add a scale condition
		rpa.Status.SetScaleCondition(len(pods.Items), rpa.Spec.Replicas)
	}

	// Set the number of replicas
	rpa.Status.Replicas = rpa.Spec.Replicas

	// Check if we need to upgrade the existing pods
	numUpdated, err := r.tryUpgradePods(rpa, pods)
	if err != nil {
		return err
	}
	if numUpdated > 0 {
		rpa.Status.SetUpgradedCondition(numUpdated, rpa.Spec.Version)
	}

	return nil
}

func (r *ReconcileRandomParagraphApp) handleService(rpa *randomv1alpha1.RandomParagraphApp) error {
	serviceName := rpa.Name + "-svc"

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: rpa.Namespace,
			Labels:    map[string]string{appLabel: appName, rpaLabel: rpa.Name},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{appLabel: appName, rpaLabel: rpa.Name},
			Type:     corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(rpa, service, r.scheme); err != nil {
		return err
	}

	// Does the service already exist
	serviceFound := &corev1.Service{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: serviceName, Namespace: rpa.Namespace}, serviceFound)
	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Info("creating service", "name", serviceName, "namespace", service.Namespace)
			errCreate := r.Create(context.TODO(), service)
			if errCreate == nil {
				r.postInfoEvent(rpa, "ServiceCreated", fmt.Sprintf("created %s/%s", serviceName, rpa.Namespace))
			}
			return errCreate
		}
		return err
	}

	//NOTE: if already existing you could check if an update is required

	return nil

}

func (r *ReconcileRandomParagraphApp) createPods(rpa *randomv1alpha1.RandomParagraphApp, numberToCreate int, existingPods *corev1.PodList) error {
	// Get the names of the existing pods
	existingNames := make([]string, len(existingPods.Items))
	for _, existingPod := range existingPods.Items {
		existingNames = append(existingNames, existingPod.Name)
	}

	maxPods := len(existingPods.Items) + numberToCreate
	for i := 0; i < numberToCreate; i++ {
		// For each pod get the next available name
		var name string
		for i := 1; i <= maxPods; i++ {
			name = fmt.Sprintf("%s-%x", rpa.Name, i)
			if !ContainsString(existingNames, name) {
				break
			}
		}

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: rpa.Namespace,
				Labels: map[string]string{
					appLabel: appName,
					rpaLabel: rpa.Name,
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{corev1.Container{
					Name:  "random-paragraph",
					Image: "richardcase/itsrandom:" + rpa.Spec.Version,
					Ports: []corev1.ContainerPort{corev1.ContainerPort{
						Name:          "http",
						Protocol:      corev1.ProtocolTCP,
						ContainerPort: 8080,
					}},
				}},
			},
		}
		if err := controllerutil.SetControllerReference(rpa, pod, r.scheme); err != nil {
			return err
		}

		r.logger.Info("creating pod", "name", pod.Name, "namespace", pod.Namespace)
		err := r.Create(context.TODO(), pod)
		if err != nil {
			return err
		}

		r.logger.Info("waiting for pod to be ready", "name", pod.Name, "namespace", pod.Namespace, "timeout", podReadinessTimeout)
		ctx, fn := context.WithTimeout(context.Background(), podReadinessTimeout)
		defer fn()
		if err := r.waitForPodReady(ctx, pod); err != nil {
			return err
		}
		r.logger.Info("pod became ready", "name", pod.Name, "namespace", pod.Namespace)

		r.postInfoEvent(rpa, "PodCreated", fmt.Sprintf("created %s/%s", pod.Name, pod.Namespace))

		existingNames = append(existingNames, pod.Name)
	}

	return nil
}

func (r *ReconcileRandomParagraphApp) deletePods(rpa *randomv1alpha1.RandomParagraphApp, podsList *corev1.PodList, numberToDelete int) error {

	sort.Slice(podsList.Items, func(i, j int) bool {
		return podsList.Items[i].CreationTimestamp.Before(&podsList.Items[j].CreationTimestamp)
	})

	for i := 0; i < numberToDelete; i++ {
		podToDelete := podsList.Items[i].DeepCopy()

		r.logger.Info("deleting pod", "name", podToDelete.Name, "namespace", podToDelete.Namespace)
		err := r.Delete(context.TODO(), podToDelete, client.GracePeriodSeconds(1))
		if err != nil {
			return err
		}

		r.logger.Info("waiting for pod to be deleted", "name", podToDelete.Name, "namespace", podToDelete.Namespace, "timeout", podDeletionTimeout)
		ctx, fn := context.WithTimeout(context.Background(), podDeletionTimeout)
		defer fn()
		if err := r.waitForPodDeletion(ctx, podToDelete); err != nil {
			return err
		}
		r.logger.Info("pod deleted", "name", podToDelete.Name, "namespace", podToDelete.Namespace)
		r.postInfoEvent(rpa, "PodDeleted", fmt.Sprintf("deleted %s/%s", podToDelete.Name, podToDelete.Namespace))
	}

	return nil
}

func (r *ReconcileRandomParagraphApp) tryUpgradePods(rpa *randomv1alpha1.RandomParagraphApp, existingPods *corev1.PodList) (int, error) {
	numUpdated := 0
	for _, pod := range existingPods.Items {
		imageParts := strings.Split(pod.Spec.Containers[0].Image, ":")
		if imageParts[1] != rpa.Spec.Version {
			// We need to upgrade a pod
			r.logger.Info("upgrading image version for pod", "name", pod.Name, "namespace", pod.Namespace, "oldversion", imageParts[1], "newversion", rpa.Spec.Version)
			err := r.upgradePod(&pod, rpa.Spec.Version)
			if err != nil {
				return 0, err
			}
			r.postInfoEvent(rpa, "PodVersionChanged", fmt.Sprintf("pod %s/%s version now %s", pod.Name, pod.Namespace, rpa.Spec.Version))
			numUpdated++
		}
	}
	return numUpdated, nil
}

func (r *ReconcileRandomParagraphApp) upgradePod(pod *corev1.Pod, version string) error {
	newVersion := "richardcase/itsrandom:" + version
	r.logger.Info("updating pod image", "name", pod.Name, "namespace", pod.Namespace, "image", newVersion)

	pod.Spec.Containers[0].Image = newVersion
	err := r.Update(context.TODO(), pod)
	if err != nil {
		return err
	}

	r.logger.Info("waiting for pod to be ready", "name", pod.Name, "namespace", pod.Namespace, "timeout", podReadinessTimeout)
	ctx, fn := context.WithTimeout(context.Background(), podReadinessTimeout)
	defer fn()
	if err := r.waitForPodReady(ctx, pod); err != nil {
		return err
	}
	r.logger.Info("pod became ready", "name", pod.Name, "namespace", pod.Namespace)

	return nil
}

func (r *ReconcileRandomParagraphApp) waitForPodReady(ctx context.Context, pod *corev1.Pod) error {
	tick := time.Tick(500 * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for pod %s to become ready", pod.Name)
		case <-tick:
			podFound := &corev1.Pod{}
			err := r.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, podFound)
			if err != nil {
				return err
			}
			if podFound.Status.Phase == corev1.PodRunning && podFound.Status.PodIP != "" {
				return nil
			}
		}
	}
}

func (r *ReconcileRandomParagraphApp) waitForPodDeletion(ctx context.Context, pod *corev1.Pod) error {
	tick := time.Tick(500 * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for pod %s to be deleted", pod.Name)
		case <-tick:
			podFound := &corev1.Pod{}
			err := r.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, podFound)
			if err != nil {
				return err
			}
			if podFound.DeletionTimestamp != nil && time.Now().After(podFound.DeletionTimestamp.Time) {
				return nil
			}
		}
	}
}

func (r *ReconcileRandomParagraphApp) postInfoEvent(obj runtime.Object, reason, message string) {
	ref, err := reference.GetReference(scheme.Scheme, obj)
	if err != nil {
		groupKind := obj.GetObjectKind().GroupVersionKind()
		r.logger.Error(err, "could not get reference for runtime object to raise event", "kind", groupKind.Kind, "group", groupKind.Group, "version", groupKind.Version)
		return
	}

	r.recorder.Event(ref, corev1.EventTypeNormal, reason, message)
}

// ContainsString checks if a string is contained in a slice
func ContainsString(sl []string, v string) bool {
	for _, vv := range sl {
		if vv == v {
			return true
		}
	}
	return false
}
