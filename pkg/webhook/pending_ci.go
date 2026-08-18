package webhook

import (
	"encoding/json"
	"fmt"
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

type Metadata struct {
	InstallationID     int64
	RepositoryID       int64
	RepositoryFullName string
	RepositoryOwner    string
	RepositoryName     string
}

type PendingCISignal struct {
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

type PendingCINotification struct {
	Event    string
	Action   string
	Key      string
	Metadata Metadata
	Signals  []PendingCISignal
}

type commonPayload struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

func SupportsPendingCI(event string) bool {
	switch event {
	case EventCheckRun, EventCheckSuite, EventStatus, EventPullRequest:
		return true
	default:
		return false
	}
}

// ParsePendingCINotification normalizes CI and pull-request deliveries into
// wake-up signals. It does not decide pending-CI state or trust the payload as
// current GitHub truth; the reconciler performs that live read later.
func ParsePendingCINotification(event string, body []byte) (*PendingCINotification, error) {
	var common commonPayload
	if err := json.Unmarshal(body, &common); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedPayload, err)
	}
	metadata, err := metadataFrom(common)
	if err != nil {
		return nil, err
	}

	var notification *PendingCINotification
	switch event {
	case EventCheckRun:
		notification, err = parseCheckRun(common, metadata, body)
	case EventCheckSuite:
		notification, err = parseCheckSuite(common, metadata, body)
	case EventStatus:
		notification, err = parseStatus(metadata, body)
	case EventPullRequest:
		notification, err = parsePullRequest(common, metadata, body)
	default:
		return nil, fmt.Errorf("%w: unsupported event %q", ErrMalformedPayload, event)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedPayload, err)
	}

	return notification, nil
}

func metadataFrom(payload commonPayload) (Metadata, error) {
	if payload.Installation.ID == 0 {
		return Metadata{}, ErrNoInstallation
	}
	if payload.Repository.ID == 0 || payload.Repository.Owner.Login == "" || payload.Repository.Name == "" {
		return Metadata{}, ErrNoRepository
	}
	fullName := payload.Repository.FullName
	if fullName == "" {
		fullName = payload.Repository.Owner.Login + "/" + payload.Repository.Name
	}

	return Metadata{
		InstallationID:     payload.Installation.ID,
		RepositoryID:       payload.Repository.ID,
		RepositoryFullName: fullName,
		RepositoryOwner:    payload.Repository.Owner.Login,
		RepositoryName:     payload.Repository.Name,
	}, nil
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
	common commonPayload,
	metadata Metadata,
	body []byte,
) (*PendingCINotification, error) {
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

	if common.Action == "requested_action" {
		return requestedCheckActionNotification(
			metadata,
			payload.CheckRun,
			payload.RequestedAction.Identifier,
			payload.Sender.Login,
		)
	}

	return checkNotification(EventCheckRun, common.Action, metadata, payload.CheckRun)
}

func requestedCheckActionNotification(
	metadata Metadata,
	subject checkSubject,
	identifier, actor string,
) (*PendingCINotification, error) {
	if subject.ID <= 0 || subject.HeadSHA == "" || subject.Name == "" ||
		subject.ExternalID == "" || subject.App.ID <= 0 || identifier == "" || actor == "" {
		return nil, fmt.Errorf("check_run requested action is missing check or actor identity")
	}
	key := fmt.Sprintf(
		"%s:%d:%d:requested_action:%s:%s",
		EventCheckRun,
		metadata.RepositoryID,
		subject.ID,
		identifier,
		actor,
	)
	signals := make([]PendingCISignal, 0, max(1, len(subject.PullRequests)))
	appendSignal := func(pullRequest int) {
		signals = append(signals, PendingCISignal{
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

	return &PendingCINotification{
		Event: EventCheckRun, Action: "requested_action", Key: key,
		Metadata: metadata, Signals: signals,
	}, nil
}

func parseCheckSuite(
	common commonPayload,
	metadata Metadata,
	body []byte,
) (*PendingCINotification, error) {
	var payload struct {
		CheckSuite checkSubject `json:"check_suite"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	return checkNotification(EventCheckSuite, common.Action, metadata, payload.CheckSuite)
}

func checkNotification(
	event, action string,
	metadata Metadata,
	subject checkSubject,
) (*PendingCINotification, error) {
	if action == "" || subject.ID == 0 || subject.HeadSHA == "" {
		return nil, fmt.Errorf("%s payload is missing action, id, or head SHA", event)
	}
	key := fmt.Sprintf(
		"%s:%d:%d:%s:%s:%s:%s",
		event,
		metadata.RepositoryID,
		subject.ID,
		action,
		subject.Status,
		subject.Conclusion,
		subject.UpdatedAt,
	)
	signals := make([]PendingCISignal, 0, max(1, len(subject.PullRequests)))
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
		signal := PendingCISignal{
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
			signals = append(signals, PendingCISignal{
				Kind: kind, HeadSHA: subject.HeadSHA, EventKey: key,
				CheckRunID: subject.ID, CheckName: subject.Name,
				ExternalID: subject.ExternalID, AppID: subject.App.ID,
			})
		} else {
			signals = append(signals, PendingCISignal{
				Kind: SignalWakeHead, HeadSHA: subject.HeadSHA, EventKey: key,
			})
		}
	}

	return &PendingCINotification{
		Event: event, Action: action, Key: key, Metadata: metadata, Signals: signals,
	}, nil
}

func parseStatus(
	metadata Metadata,
	body []byte,
) (*PendingCINotification, error) {
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
		EventStatus,
		metadata.RepositoryID,
		payload.SHA,
		payload.Context,
		payload.State,
		payload.UpdatedAt,
	)

	return &PendingCINotification{
		Event: EventStatus, Action: payload.State, Key: key, Metadata: metadata,
		Signals: []PendingCISignal{{Kind: SignalWakeHead, HeadSHA: payload.SHA, EventKey: key}},
	}, nil
}

func parsePullRequest(
	common commonPayload,
	metadata Metadata,
	body []byte,
) (*PendingCINotification, error) {
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
	if common.Action == "" || payload.Number <= 0 {
		return nil, fmt.Errorf("pull request payload is missing action or number")
	}
	key := fmt.Sprintf(
		"%s:%d:%d:%s:%s:%s:%s",
		EventPullRequest,
		metadata.RepositoryID,
		payload.Number,
		common.Action,
		payload.PullRequest.Head.SHA,
		payload.PullRequest.UpdatedAt,
		payload.Label.Name,
	)
	signal := PendingCISignal{
		Kind: SignalWakePullRequest, PullRequest: payload.Number,
		HeadSHA: payload.PullRequest.Head.SHA, EventKey: key,
	}
	switch common.Action {
	case "closed":
		signal.Kind = SignalPullRequestDone
		signal.Merged = payload.PullRequest.Merged
	case "unlabeled":
		signal.Kind = SignalLabelRemoved
		signal.Label = payload.Label.Name
	case "opened", "synchronize", "reopened", "ready_for_review", "converted_to_draft", "edited", "labeled",
		"unlocked", "enqueued", "dequeued":
	default:
		return &PendingCINotification{
			Event: EventPullRequest, Action: common.Action, Key: key, Metadata: metadata,
		}, nil
	}

	return &PendingCINotification{
		Event: EventPullRequest, Action: common.Action, Key: key, Metadata: metadata,
		Signals: []PendingCISignal{signal},
	}, nil
}
