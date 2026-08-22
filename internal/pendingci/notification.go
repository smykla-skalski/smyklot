package pendingci

import (
	"encoding/json"
	"fmt"

	"github.com/smykla-skalski/smyklot/pkg/webhook"
)

type SignalKind string

const (
	SignalWakePullRequest SignalKind = "wake_pull_request"
	SignalWakeHead        SignalKind = "wake_head"
	SignalPullRequestDone SignalKind = "pull_request_done"
	SignalLabelRemoved    SignalKind = "label_removed"
	SignalReauthorize     SignalKind = "reauthorize"
	SignalRerequestCheck  SignalKind = "rerequest_check"
)

type Signal struct {
	Kind        SignalKind
	PullRequest int
	HeadSHA     string
	MatchHead   bool
	EventKey    string
	Merged      bool
	Label       string
	Actor       string
	CheckRunID  int64
	CheckName   string
	ExternalID  string
	AppID       int64
	ActionID    string
}

type Notification struct {
	Event   string
	Action  string
	Source  webhook.Source
	Signals []Signal
}

// ParseNotification normalizes CI and pull-request deliveries into
// wake-up signals. It does not decide pending-CI state or trust the payload as
// current GitHub truth; the reconciler performs that live read later.
func ParseNotification(
	event string,
	source webhook.Source,
	body []byte,
) (*Notification, error) {
	var (
		notification *Notification
		err          error
	)
	switch event {
	case webhook.EventCheckRun:
		notification, err = parseCheckRun(source, body)
	case webhook.EventCheckSuite:
		notification, err = parseCheckSuite(source, body)
	case webhook.EventStatus:
		notification, err = parseStatus(source, body)
	case webhook.EventPullRequest:
		notification, err = parsePullRequest(source, body)
	default:
		return nil, fmt.Errorf("%w: unsupported event %q", webhook.ErrMalformedPayload, event)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", webhook.ErrMalformedPayload, err)
	}

	return notification, nil
}

type checkSubject struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ExternalID   string `json:"external_id"`
	HeadSHA      string `json:"head_sha"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	UpdatedAt    string `json:"updated_at"`
	PullRequests []struct {
		Number int `json:"number"`
	} `json:"pull_requests"`
	App struct {
		ID int64 `json:"id"`
	} `json:"app"`
}

func parseCheckRun(
	source webhook.Source,
	body []byte,
) (*Notification, error) {
	var payload struct {
		CheckRun        checkSubject `json:"check_run"`
		RequestedAction struct {
			Identifier string `json:"identifier"`
		} `json:"requested_action"`
		Sender struct {
			Login string `json:"login"`
		} `json:"sender"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	if source.Action == "requested_action" {
		return requestedCheckActionNotification(
			source,
			payload.CheckRun,
			payload.RequestedAction.Identifier,
			payload.Sender.Login,
		)
	}

	return checkNotification(webhook.EventCheckRun, source.Action, source, payload.CheckRun)
}

func requestedCheckActionNotification(
	source webhook.Source,
	subject checkSubject,
	identifier, actor string,
) (*Notification, error) {
	if subject.ID <= 0 || subject.HeadSHA == "" || subject.Name == "" ||
		subject.ExternalID == "" || subject.App.ID <= 0 || identifier == "" || actor == "" {
		return nil, fmt.Errorf("check_run requested action is missing check or actor identity")
	}
	key := fmt.Sprintf(
		"%s:%d:%d:requested_action:%s:%s",
		webhook.EventCheckRun,
		source.Repository.ID,
		subject.ID,
		identifier,
		actor,
	)
	signals := make([]Signal, 0, max(1, len(subject.PullRequests)))
	appendSignal := func(pullRequest int) {
		signals = append(signals, Signal{
			Kind: SignalReauthorize, PullRequest: pullRequest, HeadSHA: subject.HeadSHA,
			EventKey: key, Actor: actor, CheckRunID: subject.ID, CheckName: subject.Name,
			ExternalID: subject.ExternalID, AppID: subject.App.ID,
			ActionID: identifier,
		})
	}
	for _, pullRequest := range subject.PullRequests {
		if pullRequest.Number > 0 {
			appendSignal(pullRequest.Number)
		}
	}
	if len(signals) == 0 {
		appendSignal(0)
	}

	return &Notification{
		Event: webhook.EventCheckRun, Action: "requested_action",
		Source: source, Signals: signals,
	}, nil
}

func parseCheckSuite(
	source webhook.Source,
	body []byte,
) (*Notification, error) {
	var payload struct {
		CheckSuite checkSubject `json:"check_suite"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	return checkNotification(webhook.EventCheckSuite, source.Action, source, payload.CheckSuite)
}

func checkNotification(
	event, action string,
	source webhook.Source,
	subject checkSubject,
) (*Notification, error) {
	if action == "" || subject.ID == 0 || subject.HeadSHA == "" {
		return nil, fmt.Errorf("%s payload is missing action, id, or head SHA", event)
	}
	key := fmt.Sprintf(
		"%s:%d:%d:%s:%s:%s:%s",
		event,
		source.Repository.ID,
		subject.ID,
		action,
		subject.Status,
		subject.Conclusion,
		subject.UpdatedAt,
	)
	signals := make([]Signal, 0, max(1, len(subject.PullRequests)))
	kind := SignalWakePullRequest
	if action == "rerequested" {
		if subject.App.ID <= 0 {
			return nil, fmt.Errorf("%s rerequest is missing App identity", event)
		}
		kind = SignalRerequestCheck
	}
	for _, pullRequest := range subject.PullRequests {
		if pullRequest.Number <= 0 {
			continue
		}
		signal := Signal{
			Kind: kind, PullRequest: pullRequest.Number,
			HeadSHA: subject.HeadSHA, MatchHead: true, EventKey: key,
		}
		if kind == SignalRerequestCheck {
			signal.CheckRunID = subject.ID
			signal.CheckName = subject.Name
			signal.ExternalID = subject.ExternalID
			signal.AppID = subject.App.ID
		}
		signals = append(signals, signal)
	}
	if len(signals) == 0 {
		if kind == SignalRerequestCheck {
			signals = append(signals, Signal{
				Kind: kind, HeadSHA: subject.HeadSHA, EventKey: key,
				CheckRunID: subject.ID, CheckName: subject.Name,
				ExternalID: subject.ExternalID, AppID: subject.App.ID,
			})
		} else {
			signals = append(signals, Signal{
				Kind: SignalWakeHead, HeadSHA: subject.HeadSHA, EventKey: key,
			})
		}
	}

	return &Notification{
		Event: event, Action: action, Source: source, Signals: signals,
	}, nil
}

func parseStatus(
	source webhook.Source,
	body []byte,
) (*Notification, error) {
	var payload struct {
		SHA       string `json:"sha"`
		Context   string `json:"context"`
		State     string `json:"state"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.SHA == "" || payload.Context == "" || payload.State == "" {
		return nil, fmt.Errorf("status payload is missing SHA, context, or state")
	}
	key := fmt.Sprintf(
		"%s:%d:%s:%s:%s:%s",
		webhook.EventStatus,
		source.Repository.ID,
		payload.SHA,
		payload.Context,
		payload.State,
		payload.UpdatedAt,
	)

	return &Notification{
		Event: webhook.EventStatus, Action: payload.State, Source: source,
		Signals: []Signal{{Kind: SignalWakeHead, HeadSHA: payload.SHA, EventKey: key}},
	}, nil
}

func parsePullRequest(
	source webhook.Source,
	body []byte,
) (*Notification, error) {
	var payload struct {
		Number      int `json:"number"`
		PullRequest struct {
			Merged    bool   `json:"merged"`
			UpdatedAt string `json:"updated_at"`
			Head      struct {
				SHA string `json:"sha"`
			} `json:"head"`
		} `json:"pull_request"`
		Label struct {
			Name string `json:"name"`
		} `json:"label"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if source.Action == "" || payload.Number <= 0 {
		return nil, fmt.Errorf("pull request payload is missing action or number")
	}
	kind := SignalWakePullRequest
	merged := false
	label := ""
	switch source.Action {
	case "closed":
		kind, merged = SignalPullRequestDone, payload.PullRequest.Merged
	case "unlabeled":
		kind, label = SignalLabelRemoved, payload.Label.Name
	case "opened", "synchronize", "reopened", "ready_for_review", "converted_to_draft", "edited", "labeled",
		"unlocked", "enqueued", "dequeued":
	default:
		return &Notification{
			Event: webhook.EventPullRequest, Action: source.Action, Source: source,
		}, nil
	}

	return &Notification{
		Event: webhook.EventPullRequest, Action: source.Action, Source: source,
		Signals: []Signal{{
			Kind: kind, PullRequest: payload.Number,
			HeadSHA: payload.PullRequest.Head.SHA, Merged: merged, Label: label,
			EventKey: fmt.Sprintf(
				"%s:%d:%d:%s:%s:%s:%s",
				webhook.EventPullRequest,
				source.Repository.ID,
				payload.Number,
				source.Action,
				payload.PullRequest.Head.SHA,
				payload.PullRequest.UpdatedAt,
				payload.Label.Name,
			),
		}},
	}, nil
}

// CommentSequence orders actions GitHub reports against the same comment
// timestamp. An edit supersedes creation; deletion supersedes both.
func CommentSequence(action string) int {
	switch action {
	case webhook.ActionCreated:
		return 1
	case webhook.ActionEdited:
		return 2
	case webhook.ActionDeleted:
		return 3
	default:
		return 0
	}
}
