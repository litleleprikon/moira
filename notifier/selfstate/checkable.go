package selfstate

import (
	"errors"
	"github.com/moira-alert/moira"
)

const (
	templateMoreThan = "%s more than %ds. Send message."
)

// Checkable is the interface for simplified events verification.
type Checkable interface {
	Check(int64, *[]moira.NotificationEvent) error
}

// baseDelay basic structure for Checkable structures
type baseDelay struct {
	log moira.Logger
	db  moira.Database

	delay       int64
	last, count *int64
}

// Handling: Handler for structures based on a basic structure
func (check baseDelay) Handling(tml, errMsg string, interval int64, events *[]moira.NotificationEvent) {
	check.log.Errorf(tml, errMsg, interval)
	appendNotificationEvents(events, errMsg, interval)
}

// RedisDelay structure to check redis disconnect delay
type RedisDelay struct {
	baseDelay
}

// Check func to check redis disconnect delay
func (check RedisDelay) Check(nowTS int64, events *[]moira.NotificationEvent) error {
	_, err := check.db.GetChecksUpdatesCount()
	if err == nil {
		*check.last = nowTS
	}

	if *check.last < nowTS-check.delay {
		check.Handling(templateMoreThan, redisDisconnectedErrorMessage, nowTS-*check.last, events)
		return errors.New(moira.SelfStateERROR)
	}
	return nil
}

// MetricDelay structure to check metric received delay
type MetricDelay struct {
	baseDelay
}

// Check func to check metric received delay
func (check MetricDelay) Check(nowTS int64, events *[]moira.NotificationEvent) error {
	ct, _ := check.db.GetLocalTriggersToCheckCount()
	if ct == 0 {
		return nil
	}
	mc, err := check.db.GetMetricsUpdatesCount()
	if err == nil {
		if *check.count != mc {
			*check.count = mc
			*check.last = nowTS
		}
	}

	if *check.last < nowTS-check.baseDelay.delay && err == nil {
		check.Handling(templateMoreThan, filterStateErrorMessage, nowTS-*check.baseDelay.last, events)
		return errors.New(moira.SelfStateERROR)
	}
	return nil
}

// CheckDelay structure to check last check delay
type CheckDelay struct {
	baseDelay
}

// Check func to check last check delay
func (check CheckDelay) Check(nowTS int64, events *[]moira.NotificationEvent) error {
	ct, _ := check.db.GetLocalTriggersToCheckCount()
	if ct == 0 {
		return nil
	}
	cc, err := check.db.GetChecksUpdatesCount()
	if err == nil {
		if *check.count != cc {
			*check.count = cc
			*check.last = nowTS
		}
	}

	if *check.last < nowTS-check.delay && err == nil {
		check.Handling(templateMoreThan, checkerStateErrorMessage, nowTS-*check.last, events)
	}
	return nil
}

// RemoteDelay func to check last remote check delay
type RemoteDelay struct {
	baseDelay
}

// Check func to check last remote check delay
func (check RemoteDelay) Check(nowTS int64, events *[]moira.NotificationEvent) error {
	ct, _ := check.db.GetRemoteTriggersToCheckCount()
	if ct == 0 {
		return nil
	}
	rcc, err := check.db.GetRemoteChecksUpdatesCount()
	if err == nil {
		if *check.count != rcc {
			*check.count = rcc
			*check.last = nowTS
		}
	}

	if *check.last < nowTS-check.delay && err == nil {
		check.Handling(templateMoreThan, remoteCheckerStateErrorMessage, nowTS-*check.last, events)
	}
	return nil
}
