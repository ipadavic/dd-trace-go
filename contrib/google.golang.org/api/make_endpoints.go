// +build ignore

// This program generates a tree of endpoints for span tagging based on the
// API definitions in github.com/google/google-api-go-client.

package main

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"text/template"

	"github.com/yosida95/uritemplate"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/google.golang.org/api"
)

type (
	APIDefinition struct {
		ID            string                  `json"id"`
		Name          string                  `json:"name"`
		CanonicalName string                  `json:"canonicalName"`
		BaseURL       string                  `json:"baseUrl"`
		BasePath      string                  `json:"basePath"`
		Resources     map[string]*APIResource `json:"resources"`
		RootURL       string                  `json:"rootUrl"`
	}
	APIResource struct {
		Methods   map[string]*APIMethod   `json:"methods"`
		Resources map[string]*APIResource `json:"resources"`
	}
	APIMethod struct {
		ID         string `json"id"`
		Path       string `json:"path"`
		HTTPMethod string `json:"httpMethod"`
	}
)

var cnt int

func main() {
	var es []api.Endpoint

	root := filepath.Join(os.Getenv("GOPATH"), "src", "google.golang.org", "api")
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		if filepath.Ext(p) == ".json" {
			var def APIDefinition
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()

			err = json.NewDecoder(f).Decode(&def)
			if err != nil {
				return err
			}

			for _, resource := range def.Resources {
				res, err := handleResource(&def, resource)
				if err != nil {
					return err
				}
				es = append(es, res...)
			}
		}

		return nil
	})
	if err != nil {
		log.Fatalln(err)
	}

	f, err := os.Create("endpoints_gen.go")
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()

	template.Must(template.New("").Parse(tpl)).Execute(f, map[string]interface{}{
		"Endpoints": es,
	})
}

func handleResource(def *APIDefinition, resource *APIResource) ([]api.Endpoint, error) {
	var es []api.Endpoint
	if resource.Methods != nil {
		for _, method := range resource.Methods {
			mes, err := handleMethod(def, resource, method)
			if err != nil {
				return nil, err
			}
			es = append(es, mes...)
		}
	}
	if resource.Resources != nil {
		for _, child := range resource.Resources {
			res, err := handleResource(def, child)
			if err != nil {
				return nil, err
			}
			es = append(es, res...)
		}
	}
	return es, nil
}

func handleMethod(def *APIDefinition, resource *APIResource, method *APIMethod) ([]api.Endpoint, error) {
	u, err := url.Parse(def.RootURL)
	if err != nil {
		return nil, err
	}
	hostname := u.Hostname()

	path := def.BasePath + method.Path

	uritpl, err := uritemplate.New(path)
	if err != nil {
		return nil, err
	}
	return []api.Endpoint{{
		Hostname:     hostname,
		HTTPMethod:   method.HTTPMethod,
		PathTemplate: path,
		PathMatcher:  uritpl.Regexp(),
		ServiceName:  "google." + def.ID,
		ResourceName: method.ID,
	}}, nil
}

var tpl = `// Code generated by make_endpoints.go DO NOT EDIT

package api

import "regexp"

// APIEndpoints are all of the defined endpoints for the Google API
var APIEndpoints = NewTree([]Endpoint{
	{{- range .Endpoints }}
	{{ . }},
	{{- end }}
}...)

`
