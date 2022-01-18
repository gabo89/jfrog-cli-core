package project

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	artifactoryCommandsUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	artifactoryUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"gopkg.in/yaml.v2"
)

const (
	buildFileName = "build.yaml"
)

type ProjectInitCommand struct {
	projectPath string
	serverId    string
}

func NewProjectInitCommand() *ProjectInitCommand {
	return &ProjectInitCommand{}
}

func (pic *ProjectInitCommand) SetProjectPath(path string) *ProjectInitCommand {
	pic.projectPath = path
	return pic
}

func (pic *ProjectInitCommand) SetServerId(id string) *ProjectInitCommand {
	pic.serverId = id
	return pic
}

func (pic *ProjectInitCommand) Run() (err error) {
	if pic.serverId == "" {
		defaultServer, err := config.GetSpecificConfig("", true, false)
		if err != nil {
			return err
		}
		pic.serverId = defaultServer.ServerId
	}
	technologiesMap, err := pic.detectTechnologies()
	if err != nil {
		return err
	}
	// First create repositories for the detected technologies.
	for techName := range technologiesMap {
		// First create repositories for the detected technology.
		err = createDefaultReposIfNeeded(techName, pic.serverId)
		if err != nil {
			return err
		}
		err = createProjectBuildConfigs(techName, pic.projectPath, pic.serverId)
		if err != nil {
			return err
		}
	}
	// Create build config
	if err = pic.createBuildConfig(); err != nil {
		return
	}

	fmt.Println()
	err = coreutils.PrintTable("", "", pic.createSummarizeMessage(technologiesMap))
	fmt.Println()

	return
}

func (pic *ProjectInitCommand) createSummarizeMessage(technologiesMap map[coreutils.Technology]bool) string {
	return coreutils.PrintBold("This project is initialized!\n") +
		coreutils.PrintBold("The project config is stored inside the .jfrog directory.") +
		"\n\n" +
		coreutils.PrintTitle("Audit your project for security vulnerabilities by running") +
		"\n" +
		"jf audit\n\n" +
		coreutils.PrintTitle("Scan any software package on this machine for security vulnerabilities by running") +
		"\n" +
		"jf scan path/to/dir/or/package\n\n" +
		coreutils.PrintTitle("If you're using VS Code, IntelliJ IDEA, WebStorm, PyCharm, Android Studio or GoLand") +
		"\n" +
		"1. Open the IDE\n" +
		"2. Install the JFrog extension or plugin\n" +
		"3. View the JFrog panel" +
		"\n\n" +
		pic.createBuildMessage(technologiesMap) +
		coreutils.PrintTitle("Read more using this link:") +
		"\n" +
		coreutils.PrintLink(coreutils.GettingStartedGuideUrl) +
		"\n\n" +
		coreutils.GetFeedbackMessage()
}

// Return a string message, which includes all the build and deployment commands, matching the technologiesMap sent.
func (pic *ProjectInitCommand) createBuildMessage(technologiesMap map[coreutils.Technology]bool) string {
	message := ""
	for tech := range technologiesMap {
		switch tech {
		case coreutils.Maven:
			message += "jf mvn install deploy\n"
		case coreutils.Gradle:
			message += "jf gradle artifactoryP\n"
		case coreutils.Npm:
			message += "jf npm install\n"
			message += "jf npm publish\n"
		case coreutils.Go:
			message +=
				"jf go build\n" +
					"jf go-publish v1.0.0\n"
		case coreutils.Pip:
			message +=
				"jf pip install\n" +
					"jf rt u path/to/package/file default-pypi-local" +
					coreutils.PrintComment(" # Publish your pip package") +
					"\n"
		case coreutils.Pipenv:
			message +=
				"jf pipenv install\n" +
					"jf rt u path/to/package/file default-pypi-local" +
					coreutils.PrintComment(" # Publish your pipenv package") +
					"\n"
		}
	}
	if message != "" {
		message = coreutils.PrintTitle("Build the code & deploy the packages by running") +
			"\n" +
			message +
			"\n" +
			coreutils.PrintTitle("Publish the build-info to Artifactory") +
			"\n" +
			"jf rt bp\n\n"
	}
	return message
}

// Returns all detected technologies found in the project directory.
// First, try to return only the technologies that detected according to files in the root directory.
// In case no indication found in the root directory, the search continue recursively.
func (pic *ProjectInitCommand) detectTechnologies() (technologiesMap map[coreutils.Technology]bool, err error) {
	technologiesMap, err = coreutils.DetectTechnologies(pic.projectPath, false, false)
	if err != nil {
		return
	}
	// In case no technologies were detected in the root directory, try again recursively.
	if len(technologiesMap) == 0 {
		technologiesMap, err = coreutils.DetectTechnologies(pic.projectPath, false, true)
		if err != nil {
			return
		}
	}
	return
}

type BuildConfigFile struct {
	Version    int    `yaml:"version,omitempty"`
	ConfigType string `yaml:"type,omitempty"`
	BuildName  string `yaml:"name,omitempty"`
}

func (pic *ProjectInitCommand) createBuildConfig() error {
	jfrogProjectDir := filepath.Join(pic.projectPath, ".jfrog", "projects")
	if err := fileutils.CreateDirIfNotExist(jfrogProjectDir); err != nil {
		return errorutils.CheckError(err)
	}
	configFilePath := filepath.Join(jfrogProjectDir, buildFileName)
	projectDirName := filepath.Base(filepath.Dir(pic.projectPath))
	buildConfigFile := &BuildConfigFile{Version: 1, ConfigType: "build", BuildName: projectDirName}
	resBytes, err := yaml.Marshal(&buildConfigFile)
	if err != nil {
		return errorutils.CheckError(err)
	}
	return errorutils.CheckError(ioutil.WriteFile(configFilePath, resBytes, 0644))
}

func createDefaultReposIfNeeded(tech coreutils.Technology, serverId string) error {
	err := CreateDefaultLocalRepo(tech, serverId)
	if err != nil {
		return err
	}
	err = CreateDefaultRemoteRepo(tech, serverId)
	if err != nil {
		return err
	}

	return CreateDefaultVirtualRepo(tech, serverId)
}

func createProjectBuildConfigs(tech coreutils.Technology, projectPath string, serverId string) error {
	jfrogProjectDir := filepath.Join(projectPath, ".jfrog", "projects")
	if err := fileutils.CreateDirIfNotExist(jfrogProjectDir); err != nil {
		return errorutils.CheckError(err)
	}
	techName := strings.ToLower(string(tech))
	configFilePath := filepath.Join(jfrogProjectDir, techName+".yaml")
	configFile := artifactoryCommandsUtils.ConfigFile{
		Version:    artifactoryCommandsUtils.BuildConfVersion,
		ConfigType: techName,
	}
	configFile.Resolver = artifactoryUtils.Repository{ServerId: serverId}
	configFile.Deployer = artifactoryUtils.Repository{ServerId: serverId}
	switch tech {
	case coreutils.Maven:
		configFile.Resolver.ReleaseRepo = MavenVirtualDefaultName
		configFile.Resolver.SnapshotRepo = MavenVirtualDefaultName
		configFile.Deployer.ReleaseRepo = MavenVirtualDefaultName
		configFile.Deployer.SnapshotRepo = MavenVirtualDefaultName
	case coreutils.Gradle:
		configFile.Resolver.Repo = GradleVirtualDefaultName
		configFile.Deployer.Repo = GradleVirtualDefaultName
	case coreutils.Npm:
		configFile.Resolver.Repo = NpmVirtualDefaultName
		configFile.Deployer.Repo = NpmVirtualDefaultName
	case coreutils.Go:
		configFile.Resolver.Repo = GoVirtualDefaultName
		configFile.Deployer.Repo = GoVirtualDefaultName
	case coreutils.Pipenv:
		fallthrough
	case coreutils.Pip:
		configFile.Resolver.Repo = PypiVirtualDefaultName
		configFile.Deployer.Repo = PypiVirtualDefaultName
	}
	resBytes, err := yaml.Marshal(&configFile)
	if err != nil {
		return errorutils.CheckError(err)
	}

	return errorutils.CheckError(ioutil.WriteFile(configFilePath, resBytes, 0644))
}

func (pic *ProjectInitCommand) CommandName() string {
	return "project_init"
}

func (pic *ProjectInitCommand) ServerDetails() (*config.ServerDetails, error) {
	return config.GetSpecificConfig("", true, false)
}