package modelinvoker

import "context"

type CapabilityQuery struct {
	Protocol Protocol
	Endpoint string
	Model    string
}

// Provider is the complete boundary visible to the Praxis Runtime. Concrete
// provider SDK values must be translated before crossing this interface.
type Provider interface {
	ID() ProviderID
	DefaultProtocol() Protocol
	Capabilities(context.Context, CapabilityQuery) (CapabilityContract, error)
	Invoke(context.Context, Request) (Response, error)
	Stream(context.Context, Request) (Stream, error)
}
