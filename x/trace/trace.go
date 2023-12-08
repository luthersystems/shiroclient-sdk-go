package trace

const (
	// TransientKeyJaegerCollectorURI is an optional transient data key to
	// configure the Jaeger tracing collector URI.
	TransientKeyJaegerCollectorURI = "trace_jaeger_collector_endpoint"

	// TransientKeyDatasetID is an optional transient data key to
	// configure the tracing dataset ID.
	TransientKeyDatasetID = "trace_dataset"
)
