package main

import (
	"fmt"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
)

var (
	files = os.Args[1:]
)

func main() {
	if err := execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func execute() error {
	if len(files) == 0 {
		return fmt.Errorf("please pass in at least one path to a yaml file to resolve")
	}
	if err := resolveFilepaths(); err != nil {
		return fmt.Errorf("error resolving filepaths: %v", err)
	}
	for _, file := range files {
		contents, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}
		m := yaml.MapSlice{}
		if err := yaml.Unmarshal(contents, &m); err != nil {
			return err
		}
		if len(m) == 0 {
			continue
		}
		taggedImages := recursiveGetTaggedImages(m)
		resolvedImages, err := resolveTagsToDigests(taggedImages)
		if err != nil {
			return err
		}
		replacedYaml := recursiveReplaceImage(m, resolvedImages)
		updatedManifest, err := yaml.Marshal(replacedYaml)
		if err != nil {
			return err
		}
		printManifest(updatedManifest, file)
	}
	return nil
}

// resolveFilepaths first checks if a given file path exists
// If not, it tries to resolve it to an absolute path and checks if that exists
func resolveFilepaths() error {
	dir, err := getWorkingDirectory()
	if err != nil {
		return err
	}
	for index, file := range files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			fullPath := filepath.Join(dir, file)
			if _, err := os.Stat(fullPath); err != nil {
				return err
			}
			files[index] = fullPath
		}
	}
	return nil
}

// getWorkingDirectory gets the directory that the kubectl plugin was called from
func getWorkingDirectory() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("no caller information")
	}
	return path.Dir(filename), nil
}

// recursiveGetTaggedImages recursively gets all images referenced by tags
// instead of digests
func recursiveGetTaggedImages(m interface{}) []string {
	images := []string{}
	switch t := m.(type) {
	case yaml.MapSlice:
		for _, v := range t {
			images = append(images, recursiveGetTaggedImages(v)...)
		}
	case yaml.MapItem:
		v := t.Value
		switch s := v.(type) {
		case string:
			if t.Key.(string) != "image" {
				images = append(images, recursiveGetTaggedImages(v)...)
			} else {
				image := v.(string)
				_, err := name.NewDigest(image, name.WeakValidation)
				if err != nil {
					images = append(images, image)
				}
			}
		default:
			images = append(images, recursiveGetTaggedImages(s)...)
		}
	case []interface{}:
		for _, v := range t {
			images = append(images, recursiveGetTaggedImages(v)...)
		}
	}
	return images
}

// resolveTagsToDigests resolves all images specified by tag to digest
// It returns a map of the form [image:tag]:[image@sha256:digest]
func resolveTagsToDigests(images []string) (map[string]string, error) {
	resolvedImages := map[string]string{}
	for _, image := range images {
		tag, err := name.NewTag(image, name.WeakValidation)
		if err != nil {
			return nil, err
		}
		auth, err := authn.DefaultKeychain.Resolve(tag.Registry)
		if err != nil {
			return nil, err
		}
		sourceImage, err := remote.Image(tag, auth, http.DefaultTransport)
		if err != nil {
			return nil, err
		}
		digest, err := sourceImage.Digest()
		if err != nil {
			return nil, err
		}
		digestName := fmt.Sprintf("%s@sha256:%s", tag.Context(), digest.Hex)
		resolvedImages[image] = digestName
	}
	return resolvedImages, nil
}

// recursiveReplaceImage recursively replaces image:tag to the corresponding image@sha256:digest
func recursiveReplaceImage(i interface{}, replacements map[string]string) interface{} {
	var replacedInterface interface{}
	switch t := i.(type) {
	case yaml.MapSlice:
		var interfaces yaml.MapSlice
		for _, v := range t {
			r := recursiveReplaceImage(v, replacements)
			switch s := r.(type) {
			case yaml.MapSlice:
				r := recursiveReplaceImage(s, replacements)
				interfaces = append(interfaces, r.(yaml.MapItem))
			case yaml.MapItem:
				interfaces = append(interfaces, s)
			}
		}
		replacedInterface = interfaces
	case yaml.MapItem:
		k := t.Key
		v := t.Value
		switch s := v.(type) {
		case string:
			if k.(string) == "image" {
				if img, present := replacements[s]; present {
					t.Value = img
				}
			}
			return t
		default:
			return yaml.MapItem{
				Key:   k,
				Value: recursiveReplaceImage(s, replacements),
			}
		}
	case []interface{}:
		var interfaces []yaml.MapSlice
		for _, v := range t {
			r := recursiveReplaceImage(v, replacements)
			switch s := r.(type) {
			case yaml.MapSlice:
				interfaces = append(interfaces, s)
			case yaml.MapItem:
				interfaces = append(interfaces, yaml.MapSlice{s})
			}
		}
		replacedInterface = interfaces
	}
	return replacedInterface
}

// printManifest prints the final replaced kubernetes manifest to STDOUT
func printManifest(mfst []byte, file string) {
	fmt.Println()
	fmt.Println(fmt.Sprintf("--- %s ---", file))
	fmt.Println()
	fmt.Println(string(mfst))
}
