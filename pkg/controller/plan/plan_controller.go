package plan

import (
	"context"
	"fmt"
	"reflect"

	riggerv1beta1 "github.com/wantedly/rigger/pkg/apis/rigger/v1beta1"
	"github.com/wantedly/rigger/pkg/clientset"
	riggertypes "github.com/wantedly/rigger/pkg/types"
	"github.com/wantedly/rigger/pkg/util"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("plan-controller")

// Add creates a new Plan Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePlan{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("plan-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Plan
	err = c.Watch(&source.Kind{Type: &riggerv1beta1.Plan{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Secret
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &riggerv1beta1.Plan{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePlan{}

// ReconcilePlan reconciles a Plan object
type ReconcilePlan struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Plan object and makes changes based on the state read
// and what is in the Plan.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Plans and Namespaces
// +kubebuilder:rbac:groups=rigger.k8s.wantedly.com,resources=plans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rigger.k8s.wantedly.com,resources=plans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cores,resources=namespaces,verbs=get;list
func (r *ReconcilePlan) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Plan instance
	plan, planDeleted, err := util.ReconcilesFetchPlan(r, context.TODO(), request.NamespacedName)
	// Plan Deleted
	if planDeleted {
		deletedPlan, found := Cache.Load(request.NamespacedName.Name)
		if !found {
			return reconcile.Result{}, fmt.Errorf("failed to delete secret collection of deleted plan because deleted plan name '%s' is not found in cache", request.NamespacedName.Name)
		}
		log.Info(fmt.Sprintf("plan deleted [namespace:%s,name:%s]", request.NamespacedName.Namespace, request.NamespacedName.Name))
		Cache.Delete(request.NamespacedName.Name)
		dstNamespace := deletedPlan.Spec.SyncDestNamespace
		labelSelector := riggertypes.DstSecretLabelCreatedByRiggerKey + "=" + riggertypes.DstSecretLabelCreatedByRiggerValue
		err := clientset.DeleteSecretCollection(dstNamespace, nil, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to delete secret collection of deleted plan [namespace:%s,selector:%s]", dstNamespace, labelSelector)
		}
		log.Info(fmt.Sprintf("succeeded to delete secret collection of deleted plan [namespace:%s,selector:%s]", dstNamespace, labelSelector))
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to get plan %s", request.NamespacedName)
	}

	// Following is operation for Secrets.

	// Update Plan cache to avoid running Reconcile loops with old settings.
	Cache.Store(plan.Name, plan)

	newSyncTargetSecretName := plan.Spec.SyncTargetSecretName
	newSyncDestNamespace := plan.Spec.SyncDestNamespace
	newIgnoreNamespaces := plan.Spec.IgnoreNamespaces

	// Plan Cretated
	if len(plan.Status.LastSyncTargetSecretName)+len(plan.Status.LastSyncDestNamespace)+len(plan.Status.LastIgnoreNamespaces) == 0 {
		log.Info(fmt.Sprintf("plan created [namespace:%s,name:%s]", plan.Namespace, plan.Name))
		if err := SyncAllNamespaceSecrets(newSyncTargetSecretName, newSyncDestNamespace, newIgnoreNamespaces); err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to sync all namespace secrets to [destnamespace:%s,targetname:%s]", newSyncTargetSecretName, newSyncDestNamespace)
		}
		log.Info(fmt.Sprintf("succeeded to sync all namespace secrets to [destnamespace:%s,targetname:%s]", newSyncTargetSecretName, newSyncDestNamespace))
		plan.Status.LastSyncTargetSecretName = newSyncTargetSecretName
		plan.Status.LastSyncDestNamespace = newSyncDestNamespace
		plan.Status.LastIgnoreNamespaces = newIgnoreNamespaces
		if err := util.ReconcilesUpdatePlan(r, context.TODO(), plan); err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to update plan status [namespace:%s,name:%s]", plan.Namespace, plan.Name)
		}
		Cache.Store(plan.Name, plan)
		log.Info(fmt.Sprintf("succeeded to update plan status [namespace:%s,name:%s]", plan.Namespace, plan.Name))
		return reconcile.Result{}, nil
	}

	// Plan Updated
	SyncTargetSecretNameUpdated := plan.Status.LastSyncTargetSecretName != newSyncTargetSecretName
	SyncDestNamespaceUpdated := plan.Status.LastSyncDestNamespace != newSyncDestNamespace
	IgnoreNamespacesUpdated := !reflect.DeepEqual(plan.Status.LastIgnoreNamespaces, newIgnoreNamespaces)
	if !(SyncTargetSecretNameUpdated || SyncDestNamespaceUpdated || IgnoreNamespacesUpdated) {
		return reconcile.Result{}, nil
	}
	log.Info(fmt.Sprintf("plan updated [namespace:%s,name:%s]", plan.Namespace, plan.Name))
	if SyncTargetSecretNameUpdated {
		// Update SyncTargetSecretName
		// TODO(unblee): Sync new SyncTargetSecretName secrets of all namespaces to SyncDestNamespace
		//               && Delete all old SyncTargetSecretName secrets from SyncDestNamespace
		plan.Status.LastSyncTargetSecretName = newSyncTargetSecretName
	}
	if SyncDestNamespaceUpdated {
		// Update SyncDestNamespace
		// TODO(unblee): Sync SyncTargetSecretName secrets of all namespaces to new SyncDestNamespace
		//               && Delete all SyncTargetSecretName secrets from old SyncDestNamespace
		plan.Status.LastSyncDestNamespace = newSyncDestNamespace
	}
	if IgnoreNamespacesUpdated {
		// Update IgnoreNamespaces
		// TODO(unblee):
		//   added ignore namespaces: Delete all SyncTargetSecretName secrets from added IgnoreNamespaces namespace
		//   deleted ignore namespaces: Sync SyncTargetSecretName secrets of all namespaces to deleted IgnoreNamespaces namespaces
		plan.Status.LastIgnoreNamespaces = newIgnoreNamespaces
	}
	if err := util.ReconcilesUpdatePlan(r, context.TODO(), plan); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to update plan status [namespace:%s,name:%s]", plan.Namespace, plan.Name)
	}
	Cache.Store(plan.Name, plan)
	log.Info(fmt.Sprintf("succeeded to update plan status [namespace:%s,name:%s]", plan.Namespace, plan.Name))
	return reconcile.Result{}, nil
}

func SyncAllNamespaceSecrets(targetSecretName, destNamespace string, ignoreNamespaces []string) error {
	allNamespaceSecrets, err := clientset.GetAllNamespaceSecrets()
	if err != nil {
		return errors.Wrap(err, "failed to get secrets of all namespace")
	}
	for _, srcSecret := range allNamespaceSecrets {
		if srcSecret.Name != targetSecretName || util.Contains(srcSecret.Namespace, ignoreNamespaces) {
			continue
		}
		dstSecret := riggertypes.NewDstSecret(destNamespace, riggertypes.NewDstSecretName(srcSecret.Namespace, srcSecret.Name), &srcSecret)
		_, err := clientset.CreateSecret(dstSecret.Namespace, dstSecret)
		if apierrors.IsAlreadyExists(err) {
			// Overwrite the existing Secret.
			_, _ = clientset.UpdateSecret(dstSecret.Namespace, dstSecret)
			log.Info(fmt.Sprintf("succeeded to update secret [namespace:%s,name:%s]", dstSecret.Namespace, dstSecret.Name))
		} else if err != nil {
			return errors.Wrapf(err, "failed to create secret [namespace:%s,name:%s]", dstSecret.Namespace, dstSecret.Name)
		} else {
			log.Info(fmt.Sprintf("succeeded to create secret [namespace:%s,name:%s]", dstSecret.Namespace, dstSecret.Name))
		}
	}
	return nil
}
