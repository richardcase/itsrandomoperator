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
	"log"
	"sort"

	randomv1alpha1 "github.com/richardcase/itsrandomoperator/pkg/apis/random/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	appName  = "random-paragraph-ws"
	appLabel = "app"
	rpaLabel = "rpa-name"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new RandomParagraphApp Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this random.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRandomParagraphApp{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("randomparagraphapp-controller", mgr, controller.Options{Reconciler: r})
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

	//mgr.GetCache().IndexField(&corev1.Pod{}, "status.phase", func(obj runtime.Object) []string {
	//	pod := obj.(corev1.Pod)
	//})

	return nil
}

var _ reconcile.Reconciler = &ReconcileRandomParagraphApp{}

// ReconcileRandomParagraphApp reconciles a RandomParagraphApp object
type ReconcileRandomParagraphApp struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a RandomParagraphApp object and makes changes based on the state read
// and what is in the RandomParagraphApp.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=random.acme.com,resources=randomparagraphapps,verbs=get;list;watch;create;update;patch;delete
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

	return reconcile.Result{}, nil
}

func (r *ReconcileRandomParagraphApp) handlePods(rpa *randomv1alpha1.RandomParagraphApp) error {
	// Get a list of the pods owned by our CRD
	pods := &corev1.PodList{}
	labelSet := map[string]string{
		appLabel: appName,
		rpaLabel: rpa.Name,
	}
	fieldSet := map[string]string{
		"status.phase": "Running",
	}
	opts := &client.ListOptions{
		Namespace:     rpa.Namespace,
		LabelSelector: labels.SelectorFromSet(labelSet),
		FieldSelector: fields.SelectorFromSet(fieldSet),
	}
	err := r.List(context.TODO(), opts, pods)
	if err != nil {
		return err
	}

	diff := len(pods.Items) - rpa.Spec.Replicas
	if diff == 0 {
		// No action needed
		log.Printf("Current number of pods equals actual debuger of pods (%d)\n", rpa.Spec.Replicas)
		return nil
	}

	if diff < 0 {
		// Not enough replicas - create
		err = r.createPods(rpa, -diff)
		if err != nil {
			return err
		}
	}
	if diff > 0 {
		// Too many replicas - delete
		err = r.deletePods(pods.DeepCopy(), diff)
		if err != nil {
			return err
		}
	}

	//TODO: handle version changes

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
			log.Printf("Creating Service %s/%s\n", service.Namespace, service.Name)
			return r.Create(context.TODO(), service)
		}
		return err
	}

	// If the service was found update it if required
	/*if !reflect.DeepEqual(service.Spec, serviceFound.Spec) {
		serviceFound.Spec.Selector = service.Spec.Selector
		serviceFound.Spec.Type = service.Spec.Type
		serviceFound.Spec.Ports = service.Spec.Ports
		log.Printf("Updating Service %s/%s\n", service.Namespace, service.Name)
		err = r.Update(context.TODO(), serviceFound)
		if err != nil {
			return err
		}
	}*/

	return nil

}

func (r *ReconcileRandomParagraphApp) createPods(rpa *randomv1alpha1.RandomParagraphApp, numberToCreate int) error {
	for i := 0; i < numberToCreate; i++ {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: rpa.Name + "-",
				Namespace:    rpa.Namespace,
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

		log.Printf("Creating Pod \n")
		err := r.Create(context.TODO(), pod)
		if err != nil {
			return err
		}

		//NOTE: ideally you'd wait until the pod was created before moving on
	}

	return nil
}

func (r *ReconcileRandomParagraphApp) deletePods(podsList *corev1.PodList, numberToDelete int) error {

	sort.Slice(podsList.Items, func(i, j int) bool {
		return podsList.Items[i].CreationTimestamp.Before(&podsList.Items[j].CreationTimestamp)
	})

	for i := 0; i < numberToDelete; i++ {
		podToDelete := podsList.Items[i].DeepCopy()

		log.Printf("Deleting Pod %s/%s\n", podToDelete.Namespace, podToDelete.Name)
		err := r.Delete(context.TODO(), podToDelete)
		if err != nil {
			return err
		}

		//NOTE: ideally you'd wait until the pod was created before moving on
	}
	return nil
}
