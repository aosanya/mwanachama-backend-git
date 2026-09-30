package mwanachamagit

import (
	_ "embed"
	"sync"

	"github.com/aosanya/mwanachama-backend-shared/spec"
)

//go:embed git.blueprint.json
var blueprintJSON []byte

var loadBlueprint = sync.OnceValues(func() (*spec.Blueprint, error) {
	return spec.ParseBlueprint(blueprintJSON)
})

func Blueprint() (*spec.Blueprint, error) { return loadBlueprint() }

func LoadSpec(path string) (*spec.Spec, error) {
	b, err := Blueprint()
	if err != nil {
		return nil, err
	}
	return b.Load(path)
}

func ParseSpec(raw []byte) (*spec.Spec, error) {
	b, err := Blueprint()
	if err != nil {
		return nil, err
	}
	return b.Parse(raw)
}
