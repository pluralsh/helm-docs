package document

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/pluralsh/helm-docs/pkg/helm"
)

type DependencyValues struct {
	Prefix                  string
	ChartValues             *yaml.Node
	ChartValuesDescriptions map[string]helm.ChartValueDescription
}

func GetDependencyValues(root helm.ChartDocumentationInfo, allChartInfoByChartPath map[string]helm.ChartDocumentationInfo) ([]DependencyValues, error) {
	return getDependencyValuesWithPrefix(root, allChartInfoByChartPath, "")
}

func untarChartDependencies(subchart, filename string) (string, error) {
	dir, err := ioutil.TempDir("", subchart)
	if err != nil {
		return "", err
	}

	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	return filepath.Join(dir, subchart), untar(dir, f)
}

func getDependencyValuesWithPrefix(root helm.ChartDocumentationInfo, allChartInfoByChartPath map[string]helm.ChartDocumentationInfo, prefix string) ([]DependencyValues, error) {
	if len(root.Dependencies) == 0 {
		return nil, nil
	}

	result := make([]DependencyValues, 0, len(root.Dependencies))

	for _, dep := range root.Dependencies {
		searchPath := ""

		if strings.HasPrefix(dep.Repository, "file://") {
			searchPath = filepath.Join(root.ChartDirectory, strings.TrimPrefix(dep.Repository, "file://"))
		} else if dep.Repository != "" {
			chartTgzPath := filepath.Join(root.ChartDirectory, "charts", fmt.Sprintf("%s-%s.tgz", dep.Name, dep.Version))
			log.Infof("checking for tgz file %s", chartTgzPath)
			if fileExists(chartTgzPath) {
				var err error
				searchPath, err = untarChartDependencies(dep.Name, chartTgzPath)
				defer os.RemoveAll(filepath.Dir(searchPath))
				if err != nil {
					log.Warnf("Failed to untar and find dependencies %s", err)
					continue
				}
				info, err := helm.ParseChartInformation(searchPath, helm.ChartValuesDocumentationParsingConfig{})
				if err == nil {
					allChartInfoByChartPath[searchPath] = info
				}
			} else {
				log.Warnf("Chart in %q has a remote dependency %q. Dependency values will not be included.", root.ChartDirectory, dep.Name)
				continue
			}
		} else {
			searchPath = filepath.Join(root.ChartDirectory, "charts", dep.Name)
		}

		depInfo, ok := allChartInfoByChartPath[searchPath]
		if !ok {
			log.Warnf("Dependency with path %q was not found. Dependency values will not be included.", searchPath)
			continue
		}

		alias := dep.Alias
		if alias == "" {
			alias = dep.Name
		}
		depPrefix := prefix + alias

		result = append(result, DependencyValues{
			Prefix:                  depPrefix,
			ChartValues:             depInfo.ChartValues,
			ChartValuesDescriptions: depInfo.ChartValuesDescriptions,
		})

		children, err := getDependencyValuesWithPrefix(depInfo, allChartInfoByChartPath, depPrefix+".")
		if err != nil {
			return nil, err
		}

		result = append(result, children...)
	}

	return result, nil
}
