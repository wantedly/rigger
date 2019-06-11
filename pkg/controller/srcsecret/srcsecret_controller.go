package srcsecret

import (
	"context"
	"fmt"
	"reflect"

	riggerv1beta1 "github.com/wantedly/rigger/pkg/apis/rigger/v1beta1"
	"github.com/wantedly/rigger/pkg/clientset"
	planctrl "github.com/wantedly/rigger/pkg/controller/plan"
	riggertypes "github.com/wantedly/rigger/pkg/types"
	"github.com/wantedly/rigger/pkg/util"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("src-secret-controller")

// Add creates a new Secret Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSrcSecret{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("src-secret-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Secret
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSrcSecret{}

// ReconcileSecret reconciles a Secret object
type ReconcileSrcSecret struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Secret object and makes changes based on the state read
// and what is in the Secret.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Secrets
// +kubebuilder:rbac:groups=cores,resources=secrets,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileSrcSecret) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Secret instance
	srcSecret, srcSecretDeleted, err := util.ReconcilesFetchSecret(r, context.TODO(), request.NamespacedName)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get secret %s", request.NamespacedName)
	}
	srcSecretExists := !srcSecretDeleted

	// If the received Secret has been deleted, since the instance does not exist, get name and namespace from the request.
	srcSecretNamespace := request.NamespacedName.Namespace
	srcSecretName := request.NamespacedName.Name

	// If the Secret is sync target, sync the Secret to the destination.
	planctrl.Cache.Range(func(name, plan interface{}) bool {
		// Verify that the Secret is sync target.
		pl := plan.(*riggerv1beta1.Plan)
		if srcSecretName != pl.Spec.SyncTargetSecretName || util.Contains(srcSecretNamespace, pl.Spec.IgnoreNamespaces) {
			return true // continue
		}

		// Following is operation for sync target.

		dstNamespace := pl.Spec.SyncDestNamespace
		dstName := riggertypes.NewDstSecretName(srcSecretNamespace, srcSecretName)
		dstSecret, dstSecretNotFound, err := util.ReconcilesFetchSecret(r, context.TODO(), types.NamespacedName{Namespace: dstNamespace, Name: dstName.String()})
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to get secret %s/%s", dstNamespace, dstName))
			return true // continue
		}
		dstSecretExists := !dstSecretNotFound

		switch {
		case srcSecretExists && dstSecretNotFound:
			// Create destination Secret
			ds := riggertypes.NewDstSecret(dstNamespace, dstName, srcSecret)
			_, err := clientset.CreateSecret(dstNamespace, ds)
			if apierrors.IsAlreadyExists(err) {
				log.Info(fmt.Sprintf("tried to create a secret, but it already exists [namespace:%s,name:%s]", dstNamespace, dstName))
			} else if err != nil {
				log.Error(err, fmt.Sprintf("failed to create secret [namespace:%s,name:%s]", dstNamespace, dstName))
				return true // continue
			} else {
				log.Info(fmt.Sprintf("succeeded to create secret [namespace:%s,name:%s]", dstNamespace, dstName))
			}
		case srcSecretExists && dstSecretExists:
			// Update destination Secret
			if reflect.DeepEqual(srcSecret.Data, dstSecret.Data) {
				return true // continue
			}
			ds := riggertypes.NewDstSecret(dstNamespace, dstName, srcSecret)
			_, err := clientset.UpdateSecret(dstNamespace, ds)
			if apierrors.IsNotFound(err) {
				log.Info(fmt.Sprintf("tried to update a secret, but it not found [namespace:%s,name:%s]", dstNamespace, dstName))
			} else if err != nil {
				log.Error(err, fmt.Sprintf("failed to update secret [namespace:%s,name:%s]", dstNamespace, dstName))
				return true // continue
			} else {
				log.Info(fmt.Sprintf("succeeded to update secret [namespace:%s,name:%s]", dstNamespace, dstName))
			}
		case srcSecretDeleted:
			// Delete destination Secret
			err := clientset.DeleteSecret(dstNamespace, dstName.String(), nil)
			if apierrors.IsNotFound(err) {
				log.Info(fmt.Sprintf("tried to delete a secret, but it not found [namespace:%s,name:%s]", dstNamespace, dstName))
			} else if err != nil {
				log.Error(err, fmt.Sprintf("failed to delete secret [namespace:%s,name:%s]", dstNamespace, dstName))
			} else {
				log.Info(fmt.Sprintf("succeeded to delete secret [namespace:%s,name:%s]", dstNamespace, dstName))
			}
		}
		return true // continue
	})

	return reconcile.Result{}, nil
}
