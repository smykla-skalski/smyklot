package webhook

import (
	"encoding/json"
	"fmt"
)

type Repository struct {
	ID       int64
	Owner    string
	Name     string
	FullName string
}

type Source struct {
	InstallationID int64
	Repository     Repository
	Action         string
}

type sourcePayload struct {
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

func ParseSource(body []byte) (Source, error) {
	var payload sourcePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return Source{}, fmt.Errorf("%w: %w", ErrMalformedPayload, err)
	}
	if payload.Installation.ID == 0 {
		return Source{}, ErrNoInstallation
	}
	if payload.Repository.ID == 0 ||
		payload.Repository.Owner.Login == "" ||
		payload.Repository.Name == "" {
		return Source{}, ErrNoRepository
	}

	fullName := payload.Repository.FullName
	if fullName == "" {
		fullName = payload.Repository.Owner.Login + "/" + payload.Repository.Name
	}

	return Source{
		InstallationID: payload.Installation.ID,
		Repository: Repository{
			ID:       payload.Repository.ID,
			Owner:    payload.Repository.Owner.Login,
			Name:     payload.Repository.Name,
			FullName: fullName,
		},
		Action: payload.Action,
	}, nil
}
