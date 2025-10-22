package tagshandler

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/next-insurance/tagemon/internal/confighandler"
)

func SetupWithManager(mgr ctrl.Manager, config *confighandler.Config, handler *Handler) error {
	if config.TagsHandler.ViewARN == "" || handler == nil {
		return nil
	}

	interval := 30 * time.Minute
	if config.TagsHandler.Interval != nil {
		interval = *config.TagsHandler.Interval
	}

	scheduler := &Scheduler{
		mgr:      mgr,
		handler:  handler,
		config:   config,
		interval: interval,
	}

	return mgr.Add(scheduler)
}
