package application

import (
	"errors"
	"strings"
	"unicode/utf8"
)

type Attributes struct {
	InformationSystem   string   `json:"informationSystem"`
	ConnectionMethod    string   `json:"connectionMethod"`
	Domain              string   `json:"domain"`
	Location            string   `json:"location"`
	ServiceOrganization string   `json:"serviceOrganization"`
	Supervisor          string   `json:"supervisor"`
	Responsible         string   `json:"responsible"`
	OSVersion           string   `json:"osVersion"`
	OSFamily            string   `json:"osFamily"`
	Administration      []string `json:"administration"`
	PersonalData        string   `json:"personalData"`
}

func normalizeAttributes(a Attributes) (Attributes, error) {
	for _, v := range []*string{&a.InformationSystem, &a.ConnectionMethod, &a.Domain, &a.Location, &a.ServiceOrganization, &a.Supervisor, &a.Responsible, &a.OSVersion, &a.OSFamily, &a.PersonalData} {
		*v = strings.TrimSpace(*v)
		if utf8.RuneCountInString(*v) > 1000 {
			return a, errors.New("Значение дополнительного поля слишком длинное")
		}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, v := range a.Administration {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	if len(out) > 30 {
		return a, errors.New("Слишком много значений «Работа АБД»")
	}
	a.Administration = out
	return a, nil
}

// Blank import cells preserve stored attributes; explicit editors handle clearing.
func mergeAttributes(old, incoming Attributes) Attributes {
	out := incoming
	for _, field := range []struct {
		dst      *string
		previous string
	}{
		{&out.InformationSystem, old.InformationSystem}, {&out.ConnectionMethod, old.ConnectionMethod},
		{&out.Domain, old.Domain}, {&out.Location, old.Location}, {&out.ServiceOrganization, old.ServiceOrganization},
		{&out.Supervisor, old.Supervisor}, {&out.Responsible, old.Responsible}, {&out.OSVersion, old.OSVersion},
		{&out.OSFamily, old.OSFamily}, {&out.PersonalData, old.PersonalData},
	} {
		if *field.dst == "" {
			*field.dst = field.previous
		}
	}
	administration := incoming.Administration
	if len(administration) == 0 {
		administration = old.Administration
	}
	out.Administration = append([]string{}, administration...)
	return out
}
