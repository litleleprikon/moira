package selfstate

import "github.com/moira-alert/moira"

const (
	templateMoreThan = "%s more than %ds. Send message."
)

// Checkable is the interface for simplified events verification.
type Checkable interface {
	Check(int64, *[]moira.NotificationEvent) string
}

// baseDelay basic structure for Checkable structures
type baseDelay struct {
	log moira.Logger
	db  moira.Database

	delay       int64
	last, count *int64
}

// Handling: Handler for structures based on a basic structure
func (check baseDelay) Handling(tml, err string, interval int64, events *[]moira.NotificationEvent) {
	check.log.Errorf(tml, err, interval)
	appendNotificationEvents(events, err, interval)
}

type RedisDelay struct {
	baseDelay
}

// Check redis disconnect delay
func (check RedisDelay) Check(nowTS int64, events *[]moira.NotificationEvent) string {
	_, err := check.db.GetChecksUpdatesCount()
	if err == nil {
		*check.last = nowTS
	}

	if *check.last < nowTS-check.baseDelay.delay {
		check.Handling(templateMoreThan, redisDisconnectedErrorMessage, nowTS-*check.last, events)
	}
	return ""
}

type MetricDelay struct {
	baseDelay
}

// Check metric received delay
func (check MetricDelay) Check(nowTS int64, events *[]moira.NotificationEvent) string {
	mc, err := check.db.GetMetricsUpdatesCount()
	if err == nil {
		if *check.count != mc {
			*check.count = mc
			*check.last = nowTS
		}
	}

	if *check.last < nowTS-check.delay && err == nil {
		check.Handling(templateMoreThan, filterStateErrorMessage, nowTS-*check.last, events)
		return moira.SelfStateERROR
	}
	return ""
}

type CheckDelay struct {
	baseDelay
}

// Check last check delay
func (check CheckDelay) Check(nowTS int64, events *[]moira.NotificationEvent) string {
	cc, err := check.db.GetChecksUpdatesCount()
	if err == nil {
		if *check.count != cc {
			*check.count = cc
			*check.last = nowTS
		}
	}

	if *check.last < nowTS-check.delay && err == nil {
		check.Handling(templateMoreThan, checkerStateErrorMessage, nowTS-*check.last, events)
		return moira.SelfStateERROR
	}
	return ""
}

type RemoteDelay struct {
	baseDelay
}

// Check last remote check delay
func (check RemoteDelay) Check(nowTS int64, events *[]moira.NotificationEvent) string {
	rcc, err := check.db.GetRemoteChecksUpdatesCount()
	if err == nil {
		if *check.count != rcc {
			*check.count = rcc
			*check.last = nowTS
		}
	}

	if *check.last < nowTS-check.delay && err == nil {
		check.Handling(templateMoreThan, remoteCheckerStateErrorMessage, nowTS-*check.last, events)
		return moira.SelfStateERROR
	}
	return ""
}
