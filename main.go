package main

import (
	"io"
	"log"
	"os"
	"strings"

	"flag"

	"gopkg.in/yaml.v3"
)

type fixer struct {
	decoder *yaml.Decoder
	encoder *yaml.Encoder

	fromApiGroup string
	toApiGroup   string
}

func newFixer(in io.Reader, out io.Writer) *fixer {
	return &fixer{
		decoder: yaml.NewDecoder(in),
		encoder: yaml.NewEncoder(out),
	}
}

func (f *fixer) run() error {
	for {
		var rawDocument interface{}
		err := f.decoder.Decode(&rawDocument)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		apiVersion := findTextChild(rawDocument, "apiVersion")
		kind := findTextChild(rawDocument, "kind")
		if apiVersion == "apiextensions.k8s.io/v1beta1" && kind == "CustomResourceDefinition" {
			if err = f.convertCrd(rawDocument); err != nil {
				return err
			}
		} else if apiVersion == "rbac.authorization.k8s.io/v1" && (kind == "ClusterRole" || kind == "Role") {
			if err = f.convertRole(rawDocument); err != nil {
				return err
			}
		} else if strings.HasPrefix(apiVersion, f.fromApiGroup+"/") {
			suffix := apiVersion[len(f.fromApiGroup+"/"):]
			rawDocument.(map[string]interface{})["apiVersion"] = f.toApiGroup + "/" + suffix
		}

		if err = f.encoder.Encode(&rawDocument); err != nil {
			return err
		}
	}
}

func (f *fixer) convertCrd(rawDocument interface{}) error {
	metadata := findMapChild(rawDocument, "metadata")
	if metadata != nil {
		name := findTextChild(metadata, "name")
		suffix := "." + f.fromApiGroup
		if strings.HasSuffix(name, suffix) {
			metadata["name"] = name[0:len(name)-len(suffix)] + "." + f.toApiGroup
		}
	}

	spec := findMapChild(rawDocument, "spec")
	if spec != nil {
		if findTextChild(spec, "group") == f.fromApiGroup {
			spec["group"] = f.toApiGroup
		}
	}
	return nil
}

func (f *fixer) convertRole(rawDocument interface{}) error {
	rules := findSequenceChild(rawDocument, "rules")
	if rules == nil {
		return nil
	}

	for _, rule := range rules {
		apiGroups := findSequenceChild(rule, "apiGroups")
		if apiGroups != nil {
			for index, apiGroup := range apiGroups {
				s, ok := apiGroup.(string)
				if ok && s == f.fromApiGroup {
					apiGroups[index] = f.toApiGroup
				}
			}
		}
	}

	return nil
}

func findChild(node interface{}, name string) interface{} {
	o, ok := node.(map[string]interface{})
	if !ok {
		return nil
	}

	for key, value := range o {
		if key == name {
			return value
		}
	}
	return nil
}

func findTextChild(node interface{}, name string) string {
	child := findChild(node, name)
	if child == nil {
		return ""
	}
	s, ok := child.(string)
	if !ok {
		return ""
	}
	return s
}

func findSequenceChild(node interface{}, name string) []interface{} {
	child := findChild(node, name)
	if child == nil {
		return nil
	}
	a, ok := child.([]interface{})
	if !ok {
		return nil
	}
	return a
}

func findMapChild(node interface{}, name string) map[string]interface{} {
	child := findChild(node, name)
	if child == nil {
		return nil
	}
	m, ok := child.(map[string]interface{})
	if !ok {
		return nil
	}
	return m
}

func main() {
	var in io.Reader
	var out io.Writer
	var err error
	in = os.Stdin
	out = os.Stdout

	fromApiGroup := flag.String("from", "", "The API Group value to change from")
	toApiGroup := flag.String("to", "", "The API Group value to change to")
	flag.Parse()

	if *fromApiGroup == "" {
		log.Fatal("Must supply -from option")
	}
	if *toApiGroup == "" {
		log.Fatal("Must supply -to option")
	}

	f := newFixer(in, out)
	f.fromApiGroup = *fromApiGroup
	f.toApiGroup = *toApiGroup
	err = f.run()
	if err != nil {
		log.Fatal(err)
	}
}
