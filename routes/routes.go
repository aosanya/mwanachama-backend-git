package routes

import (
	"github.com/aosanya/mwanachama-backend-shared/dispatch"
	"github.com/aosanya/mwanachama-backend-shared/httpwire"

	mwanachamagit "github.com/aosanya/mwanachama-backend-git"
)

type Route = httpwire.Route

type Mount = dispatch.Mount

var Sentinels = mwanachamagit.Sentinels()

var AnonymousActions = []string{}

var Table = dispatch.NewTable(mwanachamagit.OperationsJSON(), Sentinels, AnonymousActions...)

func Build(gm mwanachamagit.GitManager) ([]Route, error) { return BuildFor(gm, Mount{}) }

func BuildFor(gm mwanachamagit.GitManager, m Mount) ([]Route, error) {
	declared, err := Table.Build(gm, m)
	if err != nil {
		return nil, err
	}
	return append(declared, undeclared(gm)...), nil
}

func Routes(gm mwanachamagit.GitManager) []Route { return RoutesFor(gm, Mount{}) }

func RoutesFor(gm mwanachamagit.GitManager, m Mount) []Route {
	return append(Table.Routes(gm, m), undeclared(gm)...)
}

func Split(gm mwanachamagit.GitManager, m Mount) dispatch.Split {
	s := Table.Split(gm, m)
	s.Gated = append(s.Gated, undeclared(gm)...)
	return s
}

func PublicRoutes(gm mwanachamagit.GitManager) []Route {
	return Table.Split(gm, Mount{}).Anonymous
}

func OperatorRoutes(gm mwanachamagit.GitManager, m Mount) []Route {
	return Split(gm, m).Gated
}
