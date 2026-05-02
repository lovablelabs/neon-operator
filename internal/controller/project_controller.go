/*
Copyright 2025.

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

package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/specs/storagecontroller"
	"oltp.molnett.org/neon-operator/utils"
)

type projectStatusInputs struct {
	TenantIDErr error
	AttachErr   error
}

// ProjectReconciler reconciles a Project object
type ProjectReconciler struct {
	client.Client
	Scheme                   *runtime.Scheme
	StorageControllerBaseURL string
}

// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=projects/finalizers,verbs=update

func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("Reconcile loop start", "request", req)
	defer func() {
		log.Info("Reconcile loop end", "request", req)
	}()

	project, err := r.getProject(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}

	if project == nil {
		return ctrl.Result{}, nil
	}

	ctx = context.WithValue(ctx, utils.ProjectNameKey, project.Name)

	result, err := r.reconcile(ctx, project)
	if errors.Is(err, ErrRequeueAfterChange) {
		return result, nil
	} else if err != nil {
		log.Error(err, "Reconcile failed")
		return ctrl.Result{}, err
	}

	return result, nil
}

func (r *ProjectReconciler) reconcile(ctx context.Context, project *neonv1alpha1.Project) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if project.Spec.TenantID == "" {
		tenantID := utils.GenerateNeonID()
		if err := r.updateTenantID(ctx, project, tenantID); err != nil {
			log.Error(err, "Failed to update tenant ID")
			if statusErr := r.updateStatus(ctx, project, projectStatusInputs{TenantIDErr: fmt.Errorf("failed to update tenant ID: %w", err)}); statusErr != nil {
				log.Error(statusErr, "failed to update project status")
			}
			return ctrl.Result{}, fmt.Errorf("failed to update tenant ID: %w", err)
		}
		log.Info("Generated and set tenant ID", "tenantID", tenantID)
		return ctrl.Result{RequeueAfter: time.Second}, ErrRequeueAfterChange
	}

	attachErr := r.ensureTenantOnPageserver(ctx, project)
	if attachErr != nil {
		log.Error(attachErr, "Failed to ensure tenant on pageserver")
	}

	if statusErr := r.updateStatus(ctx, project, projectStatusInputs{AttachErr: attachErr}); statusErr != nil {
		log.Error(statusErr, "failed to update project status")
		return ctrl.Result{}, statusErr
	}

	if attachErr != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func (r *ProjectReconciler) updateStatus(ctx context.Context, project *neonv1alpha1.Project, in projectStatusInputs) error {
	return utils.PatchStatus(ctx, r.Client, project, func(p *neonv1alpha1.Project) {
		p.Status.ObservedGeneration = p.Generation
		conds := &p.Status.Conditions

		switch {
		case in.TenantIDErr != nil:
			utils.SetCondition(p, conds, utils.ConditionTenantIDAssigned, metav1.ConditionFalse, utils.ReasonTenantIDPending, in.TenantIDErr.Error())
		case p.Spec.TenantID != "":
			utils.SetCondition(p, conds, utils.ConditionTenantIDAssigned, metav1.ConditionTrue, utils.ReasonAsExpected, "Tenant ID is assigned")
		default:
			utils.SetCondition(p, conds, utils.ConditionTenantIDAssigned, metav1.ConditionFalse, utils.ReasonTenantIDPending, "Tenant ID has not been generated yet")
		}

		switch {
		case in.TenantIDErr != nil:
			utils.SetCondition(p, conds, utils.ConditionAttached, metav1.ConditionFalse, utils.ReasonTenantIDPending, "Tenant ID assignment failed")
		case in.AttachErr != nil:
			reason := utils.ReasonAttachFailed
			if isConnectionError(in.AttachErr) {
				reason = utils.ReasonStorageControllerUnreachable
			}
			utils.SetCondition(p, conds, utils.ConditionAttached, metav1.ConditionFalse, reason, in.AttachErr.Error())
		case p.Spec.TenantID == "":
			utils.SetCondition(p, conds, utils.ConditionAttached, metav1.ConditionFalse, utils.ReasonTenantIDPending, "Tenant ID has not been assigned")
		default:
			utils.SetCondition(p, conds, utils.ConditionAttached, metav1.ConditionTrue, utils.ReasonAsExpected, "Tenant attached on storage controller")
		}

		if in.TenantIDErr == nil && in.AttachErr == nil && p.Spec.TenantID != "" {
			utils.SetCondition(p, conds, utils.ConditionAvailable, metav1.ConditionTrue, utils.ReasonAsExpected, "Project is Available")
			utils.SetCondition(p, conds, utils.ConditionProgressing, metav1.ConditionFalse, utils.ReasonAsExpected, "Project is at desired state")
		} else {
			utils.SetCondition(p, conds, utils.ConditionAvailable, metav1.ConditionFalse, utils.ReasonReconciling, "Project is not yet Available")
			utils.SetCondition(p, conds, utils.ConditionProgressing, metav1.ConditionTrue, utils.ReasonReconciling, "Working toward desired state")
		}
	})
}

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if utilnet.IsConnectionRefused(err) || utilnet.IsConnectionReset(err) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, io.EOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

func (r *ProjectReconciler) getProject(ctx context.Context, req ctrl.Request) (*neonv1alpha1.Project, error) {
	log := logf.FromContext(ctx)
	project := &neonv1alpha1.Project{}
	if err := r.Get(ctx, req.NamespacedName, project); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Project has been deleted")
			return nil, nil
		}

		return nil, fmt.Errorf("cannot get the resource: %w", err)
	}
	return project, nil
}

func (r *ProjectReconciler) updateTenantID(ctx context.Context, project *neonv1alpha1.Project, tenantID string) error {
	current := &neonv1alpha1.Project{}
	if err := r.Get(ctx, types.NamespacedName{Name: project.GetName(), Namespace: project.GetNamespace()}, current); err != nil {
		return err
	}

	updated := current.DeepCopy()
	updated.Spec.TenantID = tenantID
	updated.ManagedFields = nil

	if err := r.Patch(ctx, updated, client.MergeFrom(current), &client.PatchOptions{FieldManager: "neon-operator"}); err != nil {
		return err
	}

	project.Spec.TenantID = tenantID
	return nil
}

func (r *ProjectReconciler) ensureTenantOnPageserver(ctx context.Context, project *neonv1alpha1.Project) error {
	log := logf.FromContext(ctx)

	base := r.StorageControllerBaseURL
	if base == "" {
		base = storagecontroller.URL(project.Spec.ClusterName)
	}
	storageControllerURL := fmt.Sprintf("%s/v1/tenant/%s/location_config", base, project.Spec.TenantID)

	log.Info("Sending request to storage controller", "url", storageControllerURL)

	requestBody := []byte(`{"mode": "AttachedSingle", "generation": 1, "tenant_conf": {}}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, storageControllerURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Info("Failed to connect to storage controller, will retry", "error", err, "url", storageControllerURL)
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Error(err, "failed to close response body")
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Info("Storage controller returned error status", "status", resp.Status)
		return fmt.Errorf("pageserver returned status: %s", resp.Status)
	}

	log.Info("Successfully created tenant on storage controller")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&neonv1alpha1.Project{}).
		Named("project").
		Complete(r)
}
