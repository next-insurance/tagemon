package yacehandler

import (
	"github.com/next-insurance/tagemon/internal/confighandler"
	"github.com/next-insurance/tagemon/internal/tagshandler"
	ctrl "sigs.k8s.io/controller-runtime"
)

func SetupWithManager(mgr ctrl.Manager, config *confighandler.Config, tagsHandlerInstance *tagshandler.Handler) error {
	reconciler := &Reconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		Config:      config,
		TagsHandler: tagsHandlerInstance,
	}

	return reconciler.SetupWithManager(mgr)
}
