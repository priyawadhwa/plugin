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
	"path/filepath"
)

var (
	cwd   string
	files []string
)

func main() {
	cwd = os.Args[1]
	files = os.Args[2:]
	if len(files) == 0 {
		fmt.Println("please pass in at least one path to a yaml file to resolve")
		os.Exit(1)
	}
	if err := execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func execute() error {
	resolveFilepaths()
	for _, file := range files {
		contents, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}
		m := make(map[interface{}]interface{})
		if err := yaml.Unmarshal(contents, &m); err != nil {
			return err
		}
		if len(m) == 0 {
			continue
		}
		taggedImages := recursiveGetTaggedImages(m)
		resolvedImages, err := resolveImages(taggedImages)
		if err != nil {
			return err
		}
		recursiveReplaceImage(m, resolvedImages)
		updatedManifest, err := yaml.Marshal(m)
		if err != nil {
			return err
		}
		printManifest(updatedManifest, file)
	}
	return nil
}

func resolveFilepaths() {
	for index, file := range files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			files[index] = filepath.Join(cwd, file)
		}
	}
}

func recursiveGetTaggedImages(i interface{}) []string {
	images := []string{}
	switch t := i.(type) {
	case []interface{}:
		for _, v := range t {
			images = append(images, recursiveGetTaggedImages(v)...)
		}
	case map[interface{}]interface{}:
		for k, v := range t {
			if k.(string) != "image" {
				images = append(images, recursiveGetTaggedImages(v)...)
				continue
			}
			image := v.(string)
			_, err := name.NewDigest(image, name.WeakValidation)
			if err != nil {
				images = append(images, image)
			}
		}
	}
	return images
}

func resolveImages(images []string) (map[string]string, error) {
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

func recursiveReplaceImage(i interface{}, replacements map[string]string) {
	switch t := i.(type) {
	case []interface{}:
		for _, v := range t {
			recursiveReplaceImage(v, replacements)
		}
	case map[interface{}]interface{}:
		for k, v := range t {
			if k.(string) != "image" {
				recursiveReplaceImage(v, replacements)
				continue
			}

			image := v.(string)
			if img, present := replacements[image]; present {
				t[k] = img
			}
		}
	}
}

func printManifest(mfst []byte, file string) {
	// fmt.Println(fmt.Sprintf("------------------ %s ------------------", file))
	fmt.Println()
	fmt.Println(string(mfst))
}
