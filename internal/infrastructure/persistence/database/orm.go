package database

import (
	"fmt"
	"strings"

	ormdialect "github.com/domainry/domainry-orm/dialect"
	ormdriver "github.com/domainry/domainry-orm/driver"
	ormmysql "github.com/domainry/domainry-orm/mysql"
	ormquery "github.com/domainry/domainry-orm/query"
	ormsqlite "github.com/domainry/domainry-orm/sqlite"
)

type sqlRenderer interface {
	baseSQLRenderer
	Name() ormdialect.Name
}

type baseSQLRenderer interface {
	ormquery.Renderer
	Insert(string, []string) string
}

type namedRenderer struct {
	baseSQLRenderer
	name ormdialect.Name
}

func (renderer namedRenderer) Name() ormdialect.Name { return renderer.name }

func deliveryRenderer(driver string, renderer baseSQLRenderer) (sqlRenderer, error) {
	if renderer == nil {
		return nil, fmt.Errorf("Delivery SQL renderer is required")
	}
	parsed, err := ormdialect.Parse(strings.TrimSpace(driver))
	if err != nil {
		return nil, fmt.Errorf("Delivery database driver %q is unsupported: %w", driver, err)
	}
	return namedRenderer{baseSQLRenderer: renderer, name: parsed.Name()}, nil
}

func deliveryProfile(driver string) (ormdriver.Profile, error) {
	parsed, err := ormdialect.Parse(strings.TrimSpace(driver))
	if err != nil {
		return nil, fmt.Errorf("Delivery database driver %q is unsupported: %w", driver, err)
	}
	switch parsed.Name() {
	case ormdialect.MySQL:
		return ormmysql.NewProfile(), nil
	case ormdialect.SQLite:
		return ormsqlite.NewProfile(), nil
	default:
		return nil, fmt.Errorf("unsupported Delivery database dialect %q", driver)
	}
}
