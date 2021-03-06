// Package observe provides functions
// that help with setting tracing/metrics
// in cloud providers, mainly GCP.
package observe // import "github.com/NYTimes/gizmo/observe"

import (
	"context"
	"os"

	"cloud.google.com/go/profiler"
	traceapi "cloud.google.com/go/trace/apiv2"
	"contrib.go.opencensus.io/exporter/stackdriver"
	"contrib.go.opencensus.io/exporter/stackdriver/monitoredresource"
	"github.com/pkg/errors"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

// RegisterAndObserveGCP will initiate and register Stackdriver profiling and tracing and
// metrics in environments that pass the tests in the IsGCPEnabled function. All
// exporters will be registered using the information returned by the GetServiceInfo
// function. Tracing and metrics are enabled via OpenCensus exporters. See the OpenCensus
// documentation for instructions for registering additional spans and metrics.
func RegisterAndObserveGCP(onError func(error)) error {
	if SkipObserve() {
		return nil
	}
	if !IsGCPEnabled() {
		return errors.New("environment is not GCP enabled, no observe tools will be run")
	}

	projectID, svcName, svcVersion := GetServiceInfo()

	exp, err := NewStackdriverExporter(projectID, onError)
	if err != nil {
		return errors.Wrap(err, "unable to initiate error tracing exporter")
	}
	trace.RegisterExporter(exp)
	view.RegisterExporter(exp)

	err = profiler.Start(profiler.Config{
		ProjectID:      projectID,
		Service:        svcName,
		ServiceVersion: svcVersion,
	})
	return errors.Wrap(err, "unable to initiate profiling client")
}

// NewStackdriverExporter will return the tracing and metrics through
// the stack driver exporter, if exists in the underlying platform.
// If exporter is registered, it returns the exporter so you can register
// it and ensure to call Flush on termination.
func NewStackdriverExporter(projectID string, onErr func(error)) (*stackdriver.Exporter, error) {
	_, svcName, svcVersion := GetServiceInfo()
	opts := getSDOpts(projectID, svcName, svcVersion, onErr)
	if opts == nil {
		return nil, nil
	}
	return stackdriver.NewExporter(*opts)
}

// GoogleProjectID returns the GCP Project ID
// that can be used to instantiate various
// GCP clients such as Stack Driver.
func GoogleProjectID() string {
	return os.Getenv("GOOGLE_CLOUD_PROJECT")
}

// IsGAE tells you whether your program is running
// within the App Engine platform.
func IsGAE() bool {
	return os.Getenv("GAE_DEPLOYMENT_ID") != ""
}

// GetGAEInfo returns the GCP Project ID,
// the service, and the version of the application.
func GetGAEInfo() (projectID, service, version string) {
	return GoogleProjectID(),
		os.Getenv("GAE_SERVICE"),
		os.Getenv("GAE_VERSION")
}

// GetServiceInfo returns the GCP Project ID,
// the service name and version (GAE or through
// SERVICE_NAME/SERVICE_VERSION env vars). Note
// that SERVICE_NAME/SERVICE_VERSION are not standard but
// your application can pass them in as variables
// to be included in your trace attributes
func GetServiceInfo() (projectID, service, version string) {
	if IsGAE() {
		return GetGAEInfo()
	}
	return GoogleProjectID(), os.Getenv("SERVICE_NAME"), os.Getenv("SERVICE_VERSION")
}

// getSDOpts returns Stack Driver Options that you can pass directly
// to the OpenCensus exporter or other libraries.
func getSDOpts(projectID, service, version string, onErr func(err error)) *stackdriver.Options {
	var mr monitoredresource.Interface

	// this is so that you can export views from your local server up to SD if you wish
	creds, err := google.FindDefaultCredentials(context.Background(), traceapi.DefaultAuthScopes()...)
	if err != nil {
		return nil
	}
	canExport := IsGAE()
	if m := monitoredresource.Autodetect(); m != nil {
		mr = m
		canExport = true
	}
	if !canExport {
		return nil
	}

	return &stackdriver.Options{
		ProjectID:         projectID,
		MonitoredResource: mr,
		MonitoringClientOptions: []option.ClientOption{
			option.WithCredentials(creds),
		},
		TraceClientOptions: []option.ClientOption{
			option.WithCredentials(creds),
		},
		OnError: onErr,
		DefaultTraceAttributes: map[string]interface{}{
			"service": service,
			"version": version,
		},
	}
}

// IsGCPEnabled returns whether the running application
// is inside GCP or has access to its products.
func IsGCPEnabled() bool {
	return IsGAE() || monitoredresource.Autodetect() != nil
}

// SkipObserve checks if the GIZMO_SKIP_OBSERVE environment variable has been populated.
// This may be used along with local development to cut down on long startup times caused
// by the 'monitoredresource.Autodetect()' call in IsGCPEnabled().
func SkipObserve() bool {
	return os.Getenv("GIZMO_SKIP_OBSERVE") != ""
}
