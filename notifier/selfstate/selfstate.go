package selfstate

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/moira-alert/moira"
	"github.com/moira-alert/moira/notifier"
	w "github.com/moira-alert/moira/worker"
)

var defaultCheckInterval = time.Second * 10

const (
	redisDisconnectedErrorMessage  = "Redis disconnected"
	filterStateErrorMessage        = "Moira-Filter does not receive metrics"
	checkerStateErrorMessage       = "Moira-Checker does not check triggers"
	remoteCheckerStateErrorMessage = "Moira-Remote-Checker does not check remote triggers"
)

const selfStateLockName = "moira-self-state-monitor"
const selfStateLockTTL = time.Second * 15

// SelfCheckWorker checks what all notifier services works correctly and send message when moira don't work
type SelfCheckWorker struct {
	Logger     moira.Logger
	DB         moira.Database
	Notifier   notifier.Notifier
	Config     Config
	tomb       tomb.Tomb
	Checkables []Checkable
}

func (selfCheck *SelfCheckWorker) selfStateChecker(stop <-chan struct{}) error {
	selfCheck.Logger.Info("Moira Notifier Self State Monitor started")

	var metricsCount, checksCount, remoteChecksCount int64
	lastMetricReceivedTS := time.Now().Unix()
	redisLastCheckTS := time.Now().Unix()
	lastCheckTS := time.Now().Unix()
	lastRemoteCheckTS := time.Now().Unix()
	nextSendErrorMessage := time.Now().Unix()

	checkTicker := time.NewTicker(defaultCheckInterval)
	defer checkTicker.Stop()

	createCheckables(selfCheck, &lastMetricReceivedTS, &redisLastCheckTS, &lastCheckTS, &lastRemoteCheckTS, &metricsCount, &checksCount, &remoteChecksCount)

	for {
		select {
		case <-stop:
			selfCheck.Logger.Info("Moira Notifier Self State Monitor stopped")
			return nil
		case <-checkTicker.C:
			selfCheck.check(time.Now().Unix(), &nextSendErrorMessage)
		}
	}
}

// Start self check worker
func (selfCheck *SelfCheckWorker) Start() error {
	if !selfCheck.Config.Enabled {
		selfCheck.Logger.Debugf("Moira Self State Monitoring disabled")
		return nil
	}
	senders := selfCheck.Notifier.GetSenders()
	if err := selfCheck.Config.checkConfig(senders); err != nil {
		selfCheck.Logger.Errorf("Can't configure Moira Self State Monitoring: %s", err.Error())
		return nil
	}

	selfCheck.tomb.Go(func() error {
		w.NewWorker(
			"Moira Self State Monitoring",
			selfCheck.Logger,
			selfCheck.DB.NewLock(selfStateLockName, selfStateLockTTL),
			selfCheck.selfStateChecker,
		).Run(selfCheck.tomb.Dying())
		return nil
	})

	return nil
}

// Stop self check worker and wait for finish
func (selfCheck *SelfCheckWorker) Stop() error {
	if !selfCheck.Config.Enabled {
		return nil
	}
	senders := selfCheck.Notifier.GetSenders()
	if err := selfCheck.Config.checkConfig(senders); err != nil {
		return nil
	}

	selfCheck.tomb.Kill(nil)
	return selfCheck.tomb.Wait()
}

func (selfCheck *SelfCheckWorker) check(nowTS int64, nextSendErrorMessage *int64) {
	var events []moira.NotificationEvent

	if *nextSendErrorMessage < nowTS {
		for _, check := range selfCheck.Checkables {
			if state := check.Check(nowTS, &events); state != "" {
				selfCheck.setNotifierState(state)
			}
		}

		if notifierState, _ := selfCheck.DB.GetNotifierState(); notifierState != moira.SelfStateOK {
			selfCheck.Logger.Errorf("%s. Send message.", notifierStateErrorMessage(notifierState))
			appendNotificationEvents(&events, notifierStateErrorMessage(notifierState), 0)
		}

		if len(events) > 0 {
			eventsJSON, _ := json.Marshal(events)
			selfCheck.Logger.Errorf("Health check. Send package of %v notification events: %s", len(events), eventsJSON)
			selfCheck.sendErrorMessages(&events)
			*nextSendErrorMessage = nowTS + selfCheck.Config.NoticeIntervalSeconds
		}
	}
}

func appendNotificationEvents(events *[]moira.NotificationEvent, message string, currentValue int64) {
	val := float64(currentValue)
	event := moira.NotificationEvent{
		Timestamp: time.Now().Unix(),
		OldState:  moira.StateNODATA,
		State:     moira.StateERROR,
		Metric:    message,
		Value:     &val,
	}

	*events = append(*events, event)
}

func (selfCheck *SelfCheckWorker) sendErrorMessages(events *[]moira.NotificationEvent) {
	var sendingWG sync.WaitGroup
	for _, adminContact := range selfCheck.Config.Contacts {
		pkg := notifier.NotificationPackage{
			Contact: moira.ContactData{
				Type:  adminContact["type"],
				Value: adminContact["value"],
			},
			Trigger: moira.TriggerData{
				Name:       "Moira health check",
				ErrorValue: float64(0),
			},
			Events:     *events,
			DontResend: true,
		}
		selfCheck.Notifier.Send(&pkg, &sendingWG)
		sendingWG.Wait()
	}
}

func (selfCheck *SelfCheckWorker) setNotifierState(state string) {
	err := selfCheck.DB.SetNotifierState(state)
	if err != nil {
		selfCheck.Logger.Errorf("Can't set notifier state: %v", err)
	}
}

func notifierStateErrorMessage(state string) string {
	const template = "Moira-Notifier does not send messages. State: %v"
	return fmt.Sprintf(template, state)
}

func createCheckables(selfCheck *SelfCheckWorker, lastMetricReceivedTS, redisLastCheckTS, lastCheckTS, lastRemoteCheckTS, metricsCount, checksCount, remoteChecksCount *int64) {
	if selfCheck == nil {
		return
	}

	selfCheck.Checkables = []Checkable{}
	base := baseCheck{log: selfCheck.Logger, db: selfCheck.DB}

	if selfCheck.Config.RedisDisconnectDelaySeconds > 0 && redisLastCheckTS != nil {
		check := &RedisDisconnect{base}
		check.last, check.delay = redisLastCheckTS, selfCheck.Config.RedisDisconnectDelaySeconds
		selfCheck.Checkables = append(selfCheck.Checkables, check)
	}

	if selfCheck.Config.LastMetricReceivedDelaySeconds > 0 && lastMetricReceivedTS != nil && metricsCount != nil {
		check := &MetricReceivedDelay{base}
		check.last, check.count, check.delay = lastMetricReceivedTS, metricsCount, selfCheck.Config.LastMetricReceivedDelaySeconds
		selfCheck.Checkables = append(selfCheck.Checkables, check)
	}

	if selfCheck.Config.LastCheckDelaySeconds > 0 && lastCheckTS != nil && checksCount != nil {
		check := &CheckDelay{base}
		check.last, check.count, check.delay = lastCheckTS, checksCount, selfCheck.Config.LastCheckDelaySeconds
		selfCheck.Checkables = append(selfCheck.Checkables, check)
	}

	if selfCheck.Config.LastRemoteCheckDelaySeconds > 0 && lastRemoteCheckTS != nil && remoteChecksCount != nil {
		check := &RemoteTriggersDelay{base}
		check.last, check.count, check.delay = lastRemoteCheckTS, remoteChecksCount, selfCheck.Config.LastRemoteCheckDelaySeconds
		selfCheck.Checkables = append(selfCheck.Checkables, check)
	}
}
