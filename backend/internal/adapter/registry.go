package adapter

import (
	"fmt"

	"github.com/trushilpatel/jobradar/internal/model"
)

// Registry returns the correct adapter for a given adapter_type.
func Registry(adapterType string) (Adapter, error) {
	switch adapterType {
	case model.AdapterGreenhouse:
		return &Greenhouse{}, nil
	case model.AdapterLever:
		return &Lever{}, nil
	case model.AdapterAshby:
		return &Ashby{}, nil
	case model.AdapterWorkday:
		return &Workday{}, nil
	case model.AdapterWorkable:
		return &Workable{}, nil
	case model.AdapterChromedp:
		return &Chromedp{}, nil
	default:
		return nil, fmt.Errorf("unknown adapter type: %s", adapterType)
	}
}
