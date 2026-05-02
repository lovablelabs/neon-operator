package utils

import (
	"context"
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ReconcileSSA creates intended owned by owner if absent, or patches it via
// server-side apply when drift(current) reports it differs from intended.
func ReconcileSSA[T client.Object](
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner client.Object,
	intended T,
	drift func(current T) bool,
) error {
	log := logf.FromContext(ctx)
	kind := reflect.TypeOf(intended).Elem().Name()
	name := intended.GetName()

	if err := ctrl.SetControllerReference(owner, intended, scheme); err != nil {
		return fmt.Errorf("set controller reference for %s %q: %w", kind, name, err)
	}

	current := reflect.New(reflect.TypeOf(intended).Elem()).Interface().(T)
	getErr := c.Get(ctx, types.NamespacedName{Name: name, Namespace: intended.GetNamespace()}, current)
	if getErr != nil && !apierrors.IsNotFound(getErr) {
		return fmt.Errorf("get %s %q: %w", kind, name, getErr)
	}

	if apierrors.IsNotFound(getErr) {
		if err := c.Create(ctx, intended, &client.CreateOptions{FieldManager: FieldManager}); err != nil {
			return fmt.Errorf("create %s %q: %w", kind, name, err)
		}
		log.Info("Created", "kind", kind, "name", name)
		return nil
	}

	if !drift(current) {
		return nil
	}
	if err := c.Patch(ctx, intended, client.Apply, &client.PatchOptions{
		Force:        ptr.To(true),
		FieldManager: FieldManager,
	}); err != nil {
		return fmt.Errorf("patch %s %q: %w", kind, name, err)
	}
	log.Info("Updated", "kind", kind, "name", name)
	return nil
}
