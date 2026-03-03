// Copyright 2021 DocDB Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sethvargo/go-githubactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getEnvFunc implements [os.Getenv] for testing.
func getEnvFunc(t *testing.T, env map[string]string) func(string) string {
	t.Helper()

	return func(key string) string {
		val, ok := env[key]
		require.True(t, ok, "missing key %q", key)

		return val
	}
}

type testCase struct {
	env      map[string]string
	expected *result
}

func TestDefine(t *testing.T) {
	for name, tc := range map[string]testCase{
		"pull_request": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "main",
				"GITHUB_EVENT_NAME": "pull_request",
				"GITHUB_HEAD_REF":   "define-docker-tag",
				"GITHUB_REF_NAME":   "1/merge",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/hanzoai/docdb-eval-dev:pr-define-docker-tag",
				},
				evalImages: []string{
					"ghcr.io/hanzoai/docdb-eval:pr-define-docker-tag",
				},
				developmentImages: []string{
					"ghcr.io/hanzoai/docdb-dev:pr-define-docker-tag",
				},
				productionImages: []string{
					"ghcr.io/hanzoai/docdb-dev:pr-define-docker-tag-prod",
				},
			},
		},
		"pull_request-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "main",
				"GITHUB_EVENT_NAME": "pull_request",
				"GITHUB_HEAD_REF":   "define-docker-tag",
				"GITHUB_REF_NAME":   "1/merge",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:pr-define-docker-tag",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:pr-define-docker-tag",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:pr-define-docker-tag",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:pr-define-docker-tag-prod",
				},
			},
		},

		"pull_request/dependabot": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "main",
				"GITHUB_EVENT_NAME": "pull_request",
				"GITHUB_HEAD_REF":   "dependabot/submodules/tests/mongo-go-driver-29d768e",
				"GITHUB_REF_NAME":   "58/merge",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/hanzoai/docdb-eval-dev:pr-mongo-go-driver-29d768e",
				},
				evalImages: []string{
					"ghcr.io/hanzoai/docdb-eval:pr-mongo-go-driver-29d768e",
				},
				developmentImages: []string{
					"ghcr.io/hanzoai/docdb-dev:pr-mongo-go-driver-29d768e",
				},
				productionImages: []string{
					"ghcr.io/hanzoai/docdb-dev:pr-mongo-go-driver-29d768e-prod",
				},
			},
		},
		"pull_request/dependabot-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "main",
				"GITHUB_EVENT_NAME": "pull_request",
				"GITHUB_HEAD_REF":   "dependabot/submodules/tests/mongo-go-driver-29d768e",
				"GITHUB_REF_NAME":   "58/merge",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:pr-mongo-go-driver-29d768e",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:pr-mongo-go-driver-29d768e",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:pr-mongo-go-driver-29d768e",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:pr-mongo-go-driver-29d768e-prod",
				},
			},
		},

		"pull_request_target": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "main",
				"GITHUB_EVENT_NAME": "pull_request_target",
				"GITHUB_HEAD_REF":   "define-docker-tag",
				"GITHUB_REF_NAME":   "main",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/hanzoai/docdb-eval-dev:pr-define-docker-tag",
				},
				evalImages: []string{
					"ghcr.io/hanzoai/docdb-eval:pr-define-docker-tag",
				},
				developmentImages: []string{
					"ghcr.io/hanzoai/docdb-dev:pr-define-docker-tag",
				},
				productionImages: []string{
					"ghcr.io/hanzoai/docdb-dev:pr-define-docker-tag-prod",
				},
			},
		},
		"pull_request_target-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "main",
				"GITHUB_EVENT_NAME": "pull_request_target",
				"GITHUB_HEAD_REF":   "define-docker-tag",
				"GITHUB_REF_NAME":   "main",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:pr-define-docker-tag",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:pr-define-docker-tag",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:pr-define-docker-tag",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:pr-define-docker-tag-prod",
				},
			},
		},

		"push/main": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "main",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"hanzoai/docdb-eval-dev:main",
					"ghcr.io/hanzoai/docdb-eval-dev:main",
					"quay.io/hanzoai/docdb-eval-dev:main",
				},
				evalImages: []string{
					"hanzoai/docdb-eval:main",
					"ghcr.io/hanzoai/docdb-eval:main",
					"quay.io/hanzoai/docdb-eval:main",
				},
				developmentImages: []string{
					"hanzoai/docdb-dev:main",
					"ghcr.io/hanzoai/docdb-dev:main",
					"quay.io/hanzoai/docdb-dev:main",
				},
				productionImages: []string{
					"hanzoai/docdb-dev:main-prod",
					"ghcr.io/hanzoai/docdb-dev:main-prod",
					"quay.io/hanzoai/docdb-dev:main-prod",
				},
			},
		},
		"push/main-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "main",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:main",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:main",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:main",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:main-prod",
				},
			},
		},

		"push/main-v1": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "main-v1",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"hanzoai/docdb-eval-dev:main-v1",
					"ghcr.io/hanzoai/docdb-eval-dev:main-v1",
					"quay.io/hanzoai/docdb-eval-dev:main-v1",
				},
				evalImages: []string{
					"hanzoai/docdb-eval:main-v1",
					"ghcr.io/hanzoai/docdb-eval:main-v1",
					"quay.io/hanzoai/docdb-eval:main-v1",
				},
				developmentImages: []string{
					"hanzoai/docdb-dev:main-v1",
					"ghcr.io/hanzoai/docdb-dev:main-v1",
					"quay.io/hanzoai/docdb-dev:main-v1",
				},
				productionImages: []string{
					"hanzoai/docdb-dev:main-v1-prod",
					"ghcr.io/hanzoai/docdb-dev:main-v1-prod",
					"quay.io/hanzoai/docdb-dev:main-v1-prod",
				},
			},
		},
		"push/main-v1-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "main-v1",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:main-v1",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:main-v1",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:main-v1",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:main-v1-prod",
				},
			},
		},

		"push/release": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "releases/2.1",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"hanzoai/docdb-eval-dev:releases-2.1",
					"ghcr.io/hanzoai/docdb-eval-dev:releases-2.1",
					"quay.io/hanzoai/docdb-eval-dev:releases-2.1",
				},
				evalImages: []string{
					"hanzoai/docdb-eval:releases-2.1",
					"ghcr.io/hanzoai/docdb-eval:releases-2.1",
					"quay.io/hanzoai/docdb-eval:releases-2.1",
				},
				developmentImages: []string{
					"hanzoai/docdb-dev:releases-2.1",
					"ghcr.io/hanzoai/docdb-dev:releases-2.1",
					"quay.io/hanzoai/docdb-dev:releases-2.1",
				},
				productionImages: []string{
					"hanzoai/docdb-dev:releases-2.1-prod",
					"ghcr.io/hanzoai/docdb-dev:releases-2.1-prod",
					"quay.io/hanzoai/docdb-dev:releases-2.1-prod",
				},
			},
		},
		"push/release-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "releases/2.1",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:releases-2.1",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:releases-2.1",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:releases-2.1",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:releases-2.1-prod",
				},
			},
		},

		"push/tag/prerelease1": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "v1.26.0-beta",
				"GITHUB_REF_TYPE":   "tag",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"hanzoai/docdb-eval-dev:1.26.0-beta",
					"ghcr.io/hanzoai/docdb-eval-dev:1.26.0-beta",
					"quay.io/hanzoai/docdb-eval-dev:1.26.0-beta",
				},
				evalImages: []string{
					"hanzoai/docdb-eval:1.26.0-beta",
					"ghcr.io/hanzoai/docdb-eval:1.26.0-beta",
					"quay.io/hanzoai/docdb-eval:1.26.0-beta",
				},
				developmentImages: []string{
					"hanzoai/docdb-dev:1.26.0-beta",
					"ghcr.io/hanzoai/docdb-dev:1.26.0-beta",
					"quay.io/hanzoai/docdb-dev:1.26.0-beta",
				},
				productionImages: []string{
					"hanzoai/docdb:1.26.0-beta",
					"ghcr.io/hanzoai/docdb:1.26.0-beta",
					"quay.io/hanzoai/docdb:1.26.0-beta",
				},
			},
		},
		"push/tag/prerelease1-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "v1.26.0-beta",
				"GITHUB_REF_TYPE":   "tag",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:1.26.0-beta",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:1.26.0-beta",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:1.26.0-beta",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo:1.26.0-beta",
				},
			},
		},

		"push/tag/prerelease2": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "v2.1.0-beta.1",
				"GITHUB_REF_TYPE":   "tag",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"hanzoai/docdb-eval-dev:2.1.0-beta.1",
					"ghcr.io/hanzoai/docdb-eval-dev:2.1.0-beta.1",
					"quay.io/hanzoai/docdb-eval-dev:2.1.0-beta.1",
				},
				evalImages: []string{
					"hanzoai/docdb-eval:2.1.0-beta.1",
					"ghcr.io/hanzoai/docdb-eval:2.1.0-beta.1",
					"quay.io/hanzoai/docdb-eval:2.1.0-beta.1",
				},
				developmentImages: []string{
					"hanzoai/docdb-dev:2.1.0-beta.1",
					"ghcr.io/hanzoai/docdb-dev:2.1.0-beta.1",
					"quay.io/hanzoai/docdb-dev:2.1.0-beta.1",
				},
				productionImages: []string{
					"hanzoai/docdb:2.1.0-beta.1",
					"ghcr.io/hanzoai/docdb:2.1.0-beta.1",
					"quay.io/hanzoai/docdb:2.1.0-beta.1",
				},
			},
		},
		"push/tag/prerelease2-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "v2.1.0-beta.1",
				"GITHUB_REF_TYPE":   "tag",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:2.1.0-beta.1",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:2.1.0-beta.1",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:2.1.0-beta.1",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo:2.1.0-beta.1",
				},
			},
		},

		"push/tag/release1": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "v1.26.0",
				"GITHUB_REF_TYPE":   "tag",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"hanzoai/docdb-eval-dev:1",
					"hanzoai/docdb-eval-dev:1.26",
					"hanzoai/docdb-eval-dev:1.26.0",
					"ghcr.io/hanzoai/docdb-eval-dev:1",
					"ghcr.io/hanzoai/docdb-eval-dev:1.26",
					"ghcr.io/hanzoai/docdb-eval-dev:1.26.0",
					"quay.io/hanzoai/docdb-eval-dev:1",
					"quay.io/hanzoai/docdb-eval-dev:1.26",
					"quay.io/hanzoai/docdb-eval-dev:1.26.0",
				},
				evalImages: []string{
					"hanzoai/docdb-eval:1",
					"hanzoai/docdb-eval:1.26",
					"hanzoai/docdb-eval:1.26.0",
					"ghcr.io/hanzoai/docdb-eval:1",
					"ghcr.io/hanzoai/docdb-eval:1.26",
					"ghcr.io/hanzoai/docdb-eval:1.26.0",
					"quay.io/hanzoai/docdb-eval:1",
					"quay.io/hanzoai/docdb-eval:1.26",
					"quay.io/hanzoai/docdb-eval:1.26.0",
				},
				developmentImages: []string{
					"hanzoai/docdb-dev:1",
					"hanzoai/docdb-dev:1.26",
					"hanzoai/docdb-dev:1.26.0",
					"ghcr.io/hanzoai/docdb-dev:1",
					"ghcr.io/hanzoai/docdb-dev:1.26",
					"ghcr.io/hanzoai/docdb-dev:1.26.0",
					"quay.io/hanzoai/docdb-dev:1",
					"quay.io/hanzoai/docdb-dev:1.26",
					"quay.io/hanzoai/docdb-dev:1.26.0",
				},
				productionImages: []string{
					"hanzoai/docdb:1",
					"hanzoai/docdb:1.26",
					"hanzoai/docdb:1.26.0",
					"ghcr.io/hanzoai/docdb:1",
					"ghcr.io/hanzoai/docdb:1.26",
					"ghcr.io/hanzoai/docdb:1.26.0",
					"quay.io/hanzoai/docdb:1",
					"quay.io/hanzoai/docdb:1.26",
					"quay.io/hanzoai/docdb:1.26.0",
				},
			},
		},
		"push/tag/release1-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "v1.26.0",
				"GITHUB_REF_TYPE":   "tag",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:1",
					"ghcr.io/otherorg/otherrepo-eval-dev:1.26",
					"ghcr.io/otherorg/otherrepo-eval-dev:1.26.0",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:1",
					"ghcr.io/otherorg/otherrepo-eval:1.26",
					"ghcr.io/otherorg/otherrepo-eval:1.26.0",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:1",
					"ghcr.io/otherorg/otherrepo-dev:1.26",
					"ghcr.io/otherorg/otherrepo-dev:1.26.0",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo:1",
					"ghcr.io/otherorg/otherrepo:1.26",
					"ghcr.io/otherorg/otherrepo:1.26.0",
				},
			},
		},

		"push/tag/release2": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "v2.0.0",
				"GITHUB_REF_TYPE":   "tag",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"hanzoai/docdb-eval-dev:2",
					"hanzoai/docdb-eval-dev:2.0",
					"hanzoai/docdb-eval-dev:2.0.0",
					"hanzoai/docdb-eval-dev:latest",
					"ghcr.io/hanzoai/docdb-eval-dev:2",
					"ghcr.io/hanzoai/docdb-eval-dev:2.0",
					"ghcr.io/hanzoai/docdb-eval-dev:2.0.0",
					"ghcr.io/hanzoai/docdb-eval-dev:latest",
					"quay.io/hanzoai/docdb-eval-dev:2",
					"quay.io/hanzoai/docdb-eval-dev:2.0",
					"quay.io/hanzoai/docdb-eval-dev:2.0.0",
					"quay.io/hanzoai/docdb-eval-dev:latest",
				},
				evalImages: []string{
					"hanzoai/docdb-eval:2",
					"hanzoai/docdb-eval:2.0",
					"hanzoai/docdb-eval:2.0.0",
					"hanzoai/docdb-eval:latest",
					"ghcr.io/hanzoai/docdb-eval:2",
					"ghcr.io/hanzoai/docdb-eval:2.0",
					"ghcr.io/hanzoai/docdb-eval:2.0.0",
					"ghcr.io/hanzoai/docdb-eval:latest",
					"quay.io/hanzoai/docdb-eval:2",
					"quay.io/hanzoai/docdb-eval:2.0",
					"quay.io/hanzoai/docdb-eval:2.0.0",
					"quay.io/hanzoai/docdb-eval:latest",
				},
				developmentImages: []string{
					"hanzoai/docdb-dev:2",
					"hanzoai/docdb-dev:2.0",
					"hanzoai/docdb-dev:2.0.0",
					"hanzoai/docdb-dev:latest",
					"ghcr.io/hanzoai/docdb-dev:2",
					"ghcr.io/hanzoai/docdb-dev:2.0",
					"ghcr.io/hanzoai/docdb-dev:2.0.0",
					"ghcr.io/hanzoai/docdb-dev:latest",
					"quay.io/hanzoai/docdb-dev:2",
					"quay.io/hanzoai/docdb-dev:2.0",
					"quay.io/hanzoai/docdb-dev:2.0.0",
					"quay.io/hanzoai/docdb-dev:latest",
				},
				productionImages: []string{
					"hanzoai/docdb:2",
					"hanzoai/docdb:2.0",
					"hanzoai/docdb:2.0.0",
					"hanzoai/docdb:latest",
					"ghcr.io/hanzoai/docdb:2",
					"ghcr.io/hanzoai/docdb:2.0",
					"ghcr.io/hanzoai/docdb:2.0.0",
					"ghcr.io/hanzoai/docdb:latest",
					"quay.io/hanzoai/docdb:2",
					"quay.io/hanzoai/docdb:2.0",
					"quay.io/hanzoai/docdb:2.0.0",
					"quay.io/hanzoai/docdb:latest",
				},
			},
		},
		"push/tag/release2-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "v2.0.0",
				"GITHUB_REF_TYPE":   "tag",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:2",
					"ghcr.io/otherorg/otherrepo-eval-dev:2.0",
					"ghcr.io/otherorg/otherrepo-eval-dev:2.0.0",
					"ghcr.io/otherorg/otherrepo-eval-dev:latest",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:2",
					"ghcr.io/otherorg/otherrepo-eval:2.0",
					"ghcr.io/otherorg/otherrepo-eval:2.0.0",
					"ghcr.io/otherorg/otherrepo-eval:latest",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:2",
					"ghcr.io/otherorg/otherrepo-dev:2.0",
					"ghcr.io/otherorg/otherrepo-dev:2.0.0",
					"ghcr.io/otherorg/otherrepo-dev:latest",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo:2",
					"ghcr.io/otherorg/otherrepo:2.0",
					"ghcr.io/otherorg/otherrepo:2.0.0",
					"ghcr.io/otherorg/otherrepo:latest",
				},
			},
		},

		"push/tag/wrong": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "2.1.0", // no leading v
				"GITHUB_REF_TYPE":   "tag",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
		},
		"push/tag/wrong-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "push",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "2.1.0", // no leading v
				"GITHUB_REF_TYPE":   "tag",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
		},

		"schedule": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "schedule",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "main",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"hanzoai/docdb-eval-dev:main",
					"ghcr.io/hanzoai/docdb-eval-dev:main",
					"quay.io/hanzoai/docdb-eval-dev:main",
				},
				evalImages: []string{
					"hanzoai/docdb-eval:main",
					"ghcr.io/hanzoai/docdb-eval:main",
					"quay.io/hanzoai/docdb-eval:main",
				},
				developmentImages: []string{
					"hanzoai/docdb-dev:main",
					"ghcr.io/hanzoai/docdb-dev:main",
					"quay.io/hanzoai/docdb-dev:main",
				},
				productionImages: []string{
					"hanzoai/docdb-dev:main-prod",
					"ghcr.io/hanzoai/docdb-dev:main-prod",
					"quay.io/hanzoai/docdb-dev:main-prod",
				},
			},
		},
		"schedule-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "schedule",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "main",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:main",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:main",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:main",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:main-prod",
				},
			},
		},

		"workflow_run": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "workflow_run",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "main",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "hanzoai/documentdb",
			},
			expected: &result{
				evalDevImages: []string{
					"hanzoai/docdb-eval-dev:main",
					"ghcr.io/hanzoai/docdb-eval-dev:main",
					"quay.io/hanzoai/docdb-eval-dev:main",
				},
				evalImages: []string{
					"hanzoai/docdb-eval:main",
					"ghcr.io/hanzoai/docdb-eval:main",
					"quay.io/hanzoai/docdb-eval:main",
				},
				developmentImages: []string{
					"hanzoai/docdb-dev:main",
					"ghcr.io/hanzoai/docdb-dev:main",
					"quay.io/hanzoai/docdb-dev:main",
				},
				productionImages: []string{
					"hanzoai/docdb-dev:main-prod",
					"ghcr.io/hanzoai/docdb-dev:main-prod",
					"quay.io/hanzoai/docdb-dev:main-prod",
				},
			},
		},
		"workflow_run-other": {
			env: map[string]string{
				"GITHUB_BASE_REF":   "",
				"GITHUB_EVENT_NAME": "workflow_run",
				"GITHUB_HEAD_REF":   "",
				"GITHUB_REF_NAME":   "main",
				"GITHUB_REF_TYPE":   "branch",
				"GITHUB_REPOSITORY": "OtherOrg/OtherRepo",
			},
			expected: &result{
				evalDevImages: []string{
					"ghcr.io/otherorg/otherrepo-eval-dev:main",
				},
				evalImages: []string{
					"ghcr.io/otherorg/otherrepo-eval:main",
				},
				developmentImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:main",
				},
				productionImages: []string{
					"ghcr.io/otherorg/otherrepo-dev:main-prod",
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			actual, err := define(getEnvFunc(t, tc.env))
			if tc.expected == nil {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestImageURL(t *testing.T) {
	// expected URLs should work
	assert.Equal(
		t,
		"https://ghcr.io/hanzoai/docdb-eval:pr-define-docker-tag",
		imageURL("ghcr.io/hanzoai/docdb-eval:pr-define-docker-tag"),
	)
	assert.Equal(
		t,
		"https://quay.io/hanzoai/docdb-eval:pr-define-docker-tag",
		imageURL("quay.io/hanzoai/docdb-eval:pr-define-docker-tag"),
	)
	assert.Equal(
		t,
		"https://hub.docker.com/r/hanzoai/docdb-eval/tags",
		imageURL("hanzoai/docdb-eval:pr-define-docker-tag"),
	)
}

func TestResults(t *testing.T) {
	dir := t.TempDir()

	summaryF, err := os.CreateTemp(dir, "summary")
	require.NoError(t, err)
	defer summaryF.Close() //nolint:errcheck // temporary file for testing

	outputF, err := os.CreateTemp(dir, "output")
	require.NoError(t, err)
	defer outputF.Close() //nolint:errcheck // temporary file for testing

	var stdout bytes.Buffer
	getenv := getEnvFunc(t, map[string]string{
		"GITHUB_STEP_SUMMARY": summaryF.Name(),
		"GITHUB_OUTPUT":       outputF.Name(),
	})
	action := githubactions.New(githubactions.WithGetenv(getenv), githubactions.WithWriter(&stdout))

	result := &result{
		evalDevImages: []string{
			"hanzoai/docdb-eval-dev:2.1.0",
		},
		evalImages: []string{
			"hanzoai/docdb-eval:2",
		},
		developmentImages: []string{
			"ghcr.io/hanzoai/docdb-dev:2",
		},
		productionImages: []string{
			"quay.io/hanzoai/docdb:latest",
		},
	}

	setResults(action, result)

	expectedStdout := strings.ReplaceAll(`
 |Type                   |Image                                                                                          |
 |----                   |-----                                                                                          |
 |Evaluation Development |['hanzoai/docdb-eval-dev:2.1.0'](https://hub.docker.com/r/hanzoai/docdb-eval-dev/tags) |
 |Evaluation             |['hanzoai/docdb-eval:2'](https://hub.docker.com/r/hanzoai/docdb-eval/tags)             |
 |Development            |['ghcr.io/hanzoai/docdb-dev:2'](https://ghcr.io/hanzoai/docdb-dev:2)                   |
 |Production             |['quay.io/hanzoai/docdb:latest'](https://quay.io/hanzoai/docdb:latest)                 |

`[1:], "'", "`",
	)
	assert.Equal(t, expectedStdout, stdout.String(), "stdout does not match")

	expectedSummary := strings.ReplaceAll(`
 |Type                   |Image                                                                                          |
 |----                   |-----                                                                                          |
 |Evaluation Development |['hanzoai/docdb-eval-dev:2.1.0'](https://hub.docker.com/r/hanzoai/docdb-eval-dev/tags) |
 |Evaluation             |['hanzoai/docdb-eval:2'](https://hub.docker.com/r/hanzoai/docdb-eval/tags)             |
 |Development            |['ghcr.io/hanzoai/docdb-dev:2'](https://ghcr.io/hanzoai/docdb-dev:2)                   |
 |Production             |['quay.io/hanzoai/docdb:latest'](https://quay.io/hanzoai/docdb:latest)                 |

`[1:], "'", "`",
	)
	b, err := io.ReadAll(summaryF)
	require.NoError(t, err)
	assert.Equal(t, expectedSummary, string(b), "summary does not match")

	expectedOutput := `
eval_dev_images<<_GitHubActionsFileCommandDelimeter_
hanzoai/docdb-eval-dev:2.1.0
_GitHubActionsFileCommandDelimeter_
eval_images<<_GitHubActionsFileCommandDelimeter_
hanzoai/docdb-eval:2
_GitHubActionsFileCommandDelimeter_
development_images<<_GitHubActionsFileCommandDelimeter_
ghcr.io/hanzoai/docdb-dev:2
_GitHubActionsFileCommandDelimeter_
production_images<<_GitHubActionsFileCommandDelimeter_
quay.io/hanzoai/docdb:latest
_GitHubActionsFileCommandDelimeter_
`[1:]
	b, err = io.ReadAll(outputF)
	require.NoError(t, err)
	assert.Equal(t, expectedOutput, string(b), "output parameters does not match")
}
