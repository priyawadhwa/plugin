package main

import (
	"fmt"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/docker"
	"github.com/sirupsen/logrus"
	"os"
)

func main() {
	if err := execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func execute() error {

	return nil
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
			parsed, err := docker.ParseReference(image)
			if err != nil {
				logrus.Warnf("Couldn't parse image: %s", v)
				continue
			}
			images = append(images, parsed.BaseName)
		}
	}
	return images
}
