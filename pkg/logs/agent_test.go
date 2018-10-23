// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/sender/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

type AgentTestSuite struct {
	suite.Suite
	testDir     string
	testLogFile string
	fakeLogs    int64

	source *config.LogSource
}

func (suite *AgentTestSuite) SetupTest() {
	var err error

	suite.testDir, err = ioutil.TempDir("", "tests")
	suite.NoError(err)

	suite.testLogFile = fmt.Sprintf("%s/test.log", suite.testDir)
	fd, err := os.Create(suite.testLogFile)
	suite.NoError(err)

	fd.WriteString("test log1\n test log2\n")
	suite.fakeLogs = 2 // Two lines.
	fd.Close()

	logConfig := config.LogsConfig{
		Type:       config.FileType,
		Path:       suite.testLogFile,
		Identifier: "test", // As it was from service-discovery to force the tailer to read from the start.
	}
	suite.source = config.NewLogSource("", &logConfig)

	config.LogsAgent.Set("logs_config.run_path", suite.testDir)
	// Shorter grace period for tests.
	config.LogsAgent.Set("logs_config.stop_grace_period", 1)
}

func (suite *AgentTestSuite) TearDownTest() {
	os.Remove(suite.testDir)

	// Resets the metrics we check.
	metrics.LogsCollected.Set(0)
	metrics.LogsProcessed.Set(0)
	metrics.LogsSent.Set(0)
	metrics.DestinationErrors.Set(0)
}

func createAgent(endpoints *config.Endpoints) (*Agent, *config.LogSources, *service.Services) {
	// setup the sources and the services
	sources := config.NewLogSources()
	services := service.NewServices()

	// setup and start the agent
	agent = NewAgent(sources, services, endpoints)
	return agent, sources, services
}

func (suite *AgentTestSuite) TestAgent() {
	l := mock.NewMockLogsIntake(suite.T())
	defer l.Close()

	endpoint := sender.AddrToEndPoint(l.Addr())
	endpoints := config.NewEndpoints(endpoint, nil)

	agent, sources, _ := createAgent(endpoints)

	zero := int64(0)
	assert.Equal(suite.T(), zero, metrics.LogsCollected.Value())
	assert.Equal(suite.T(), zero, metrics.LogsProcessed.Value())
	assert.Equal(suite.T(), zero, metrics.LogsSent.Value())
	assert.Equal(suite.T(), zero, metrics.DestinationErrors.Value())

	agent.Start()
	sources.AddSource(suite.source)
	// Give the tailer some time to start its job.
	time.Sleep(10 * time.Millisecond)
	agent.Stop()

	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsCollected.Value())
	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsProcessed.Value())
	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsSent.Value())
	assert.Equal(suite.T(), zero, metrics.DestinationErrors.Value())

	// Validate that we can restart it without obvious breakages.
	agent.Start()
	agent.Stop()
}

func (suite *AgentTestSuite) TestAgentStopsWithWrongBackend() {
	endpoint := config.Endpoint{Host: "fake:", Port: 0}
	endpoints := config.NewEndpoints(endpoint, nil)

	agent, sources, _ := createAgent(endpoints)

	agent.Start()
	sources.AddSource(suite.source)
	// Give the tailer some time to start its job.
	time.Sleep(10 * time.Millisecond)
	agent.Stop()

	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsCollected.Value())
	assert.Equal(suite.T(), suite.fakeLogs, metrics.LogsProcessed.Value())
	assert.Equal(suite.T(), int64(0), metrics.LogsSent.Value())
	assert.True(suite.T(), metrics.DestinationErrors.Value() > 0)
}

func TestAgentTestSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}
