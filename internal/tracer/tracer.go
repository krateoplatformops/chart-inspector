package tracer

import (
	"net/http"
	"net/http/httputil"
	"strings"
)

// Tracer implements http.RoundTripper.  It prints each request and
// response/error to t.OutFile.  WARNING: this may output sensitive information
// including bearer tokens.
type Tracer struct {
	http.RoundTripper
	resources []Resource
}

func (t *Tracer) GetResources() []Resource {
	return t.resources
}

func (t *Tracer) WithRoundTripper(rt http.RoundTripper) *Tracer {
	t.RoundTripper = rt
	return t
}

type Resource struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// RoundTrip calls the nested RoundTripper while printing each request and
// response/error to t.OutFile on either side of the nested call.  WARNING: this
// may output sensitive information including bearer tokens.
func (t *Tracer) RoundTrip(req *http.Request) (*http.Response, error) {
	// Dump the request to t.OutFile.
	_, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		return nil, err
	}
	// os.Stderr.Write(b)
	// os.Stderr.Write([]byte{'\n'})

	split := strings.Split(req.URL.Path, "/")
	if len(split) > 2 {
		if len(split) > 6 && split[1] == "apis" && split[3] == "namespaces" {
			t.resources = append(t.resources, Resource{
				Group:     "",
				Version:   split[2],
				Resource:  split[5],
				Namespace: split[4],
				Name:      split[6],
			})
		} else if len(split) > 7 && split[1] == "apis" && split[4] == "namespaces" {
			t.resources = append(t.resources, Resource{
				Group:     split[2],
				Version:   split[3],
				Resource:  split[6],
				Namespace: split[5],
				Name:      split[7],
			})
		} else if len(split) > 6 && split[1] == "api" && split[3] == "namespaces" {
			t.resources = append(t.resources, Resource{
				Group:     "",
				Version:   split[2],
				Resource:  split[5],
				Namespace: split[4],
				Name:      split[6],
			})
		} else if len(split) > 7 && split[1] == "api" && split[4] == "namespaces" {
			t.resources = append(t.resources, Resource{
				Group:     split[2],
				Version:   split[3],
				Resource:  split[6],
				Namespace: split[5],
				Name:      split[7],
			})
		}
	}

	// Call the nested RoundTripper.
	resp, err := t.RoundTripper.RoundTrip(req)
	// If an error was returned, dump it to t.OutFile.
	if err != nil {
		// fmt.Fprintln(t.OutFile, err)
		return resp, err
	}

	//Dump the response to t.OutFile.
	_, err = httputil.DumpResponse(resp, req.URL.Query().Get("watch") != "true")
	if err != nil {
		return nil, err
	}
	// os.Stderr.Write(b)
	// os.Stderr.Write([]byte{'\n'})

	return resp, err
}
