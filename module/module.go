// Package module exposes the in-process Delivery factory.
package module

import (
	deliverysdk "github.com/domainry/domainry-delivery-sdk"

	moduleassembly "github.com/domainry/domainry-delivery/internal/assembly/module"
)

func NewFactory() deliverysdk.Factory { return moduleassembly.NewFactory() }
