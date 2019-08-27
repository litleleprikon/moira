package selfstate

import "github.com/moira-alert/moira"

const (
	templateMoreThan = "%s more than %ds. Send message."
)

type Checkable interface {
	Check(int64, []moira.NotificationEvent) string
}

type baseCheck struct {
	log moira.Logger
	db  moira.Database

	delay       int64
	last, count *int64
}

func (check baseCheck) Handle(tml, err string, interval int64, events []moira.NotificationEvent) {
	check.log.Errorf(tml, err, interval)
	appendNotificationEvents(events, err, interval)
}

type RedisDisconnect struct {
	baseCheck
}

func (check RedisDisconnect) Check(nowTS int64, events []moira.NotificationEvent) string {
	_, err := check.db.GetChecksUpdatesCount()
	if err == nil {
		*check.last = nowTS
	}

	if *check.last < nowTS-check.delay {
		check.Handle(templateMoreThan, redisDisconnectedErrorMessage, nowTS-*check.last, events)
	}
	return ""
}

type MetricReceivedDelay struct {
	baseCheck
}

func (check MetricReceivedDelay) Check(nowTS int64, events []moira.NotificationEvent) string {
	mc, err := check.db.GetMetricsUpdatesCount()
	if err == nil {
		if *check.count != mc {
			*check.count = mc
			*check.last = nowTS
		}
	}

	if *check.last < nowTS-check.delay && err == nil {
		check.Handle(templateMoreThan, filterStateErrorMessage, nowTS-*check.last, events)
		return moira.SelfStateERROR
	}
	return ""
}

type CheckDelay struct {
	baseCheck
}

func (check CheckDelay) Check(nowTS int64, events []moira.NotificationEvent) string {
	cc, err := check.db.GetChecksUpdatesCount()
	if err == nil {
		if *check.count != cc {
			*check.count = cc
			*check.last = nowTS
		}
	}

	if *check.last < nowTS-check.delay && err == nil {
		check.Handle(templateMoreThan, checkerStateErrorMessage, nowTS-*check.last, events)
		return moira.SelfStateERROR
	}
	return ""
}

type RemoteTriggersDelay struct {
	baseCheck
}

func (check RemoteTriggersDelay) Check(nowTS int64, events []moira.NotificationEvent) string {
	rcc, err := check.db.GetRemoteChecksUpdatesCount()
	if err == nil {
		if *check.count != rcc {
			*check.count = rcc
			*check.last = nowTS
		}
	}

	if *check.last < nowTS-check.delay && err == nil {
		check.Handle(templateMoreThan, remoteCheckerStateErrorMessage, nowTS-*check.last, events)
		return moira.SelfStateERROR
	}
	return ""
}
