package webhook

import "encoding/json"

// Repository identifies where a delivery came from.
type Repository struct {
	ID       int64
	Owner    string
	Name     string
	FullName string
}

// Source is what every App delivery names, whatever the event: the
// installation to mint a token for, the repository it happened in, and what
// happened.
//
// It is read from the body rather than from headers because the signature
// covers the body and does not cover the headers.
type Source struct {
	InstallationID int64
	Repository     Repository

	// Action is the payload's top-level action. Some events have none - status
	// is one - and those leave it empty rather than inventing a value.
	Action string
}

// sourcePayload is the part of any App delivery this reads. Everything else is
// left unparsed: a struct that decoded more would have to be right about more.
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

// ParseSource reads the installation and repository out of any delivery.
//
// A delivery carrying no installation is refused: there is nothing to mint a
// token for, so there is nothing an App could do with it. A delivery carrying
// no repository is refused too - organization-level events have none, but no
// event this is asked about is one, and accepting a zero repository would give
// every such delivery the same identity.
//
// GitHub sometimes omits full_name where it sends owner and name, so it is
// composed rather than trusted.
func ParseSource(body []byte) (Source, error) {
	var payload sourcePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return Source{}, ErrMalformedPayload
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
