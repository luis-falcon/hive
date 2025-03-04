/*
Copyright (C) 2019 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package unreachable

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
)

const (
	testName      = "test-cluster-deployment"
	testNamespace = "test-namespace"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name                          string
		cd                            *hivev1.ClusterDeployment
		errorConnecting               *bool
		errorConnectingSecondary      *bool
		expectedStatus                corev1.ConditionStatus
		expectActiveOverrideCondition bool
		expectedActiveOverrideStatus  corev1.ConditionStatus
		expectRequeue                 bool
		expectRequeueAfter            bool
	}{
		{
			name:               "recent reachable condition",
			cd:                 buildClusterDeployment(withUnreachableCondition(corev1.ConditionFalse, time.Now())),
			expectedStatus:     corev1.ConditionFalse,
			expectRequeueAfter: true,
		},
		{
			name:            "unreachable with no condition",
			cd:              buildClusterDeployment(),
			errorConnecting: pointer.BoolPtr(true),
			expectedStatus:  corev1.ConditionTrue,
			expectRequeue:   true,
		},
		{
			name:            "unreachable with old reachable condition",
			cd:              buildClusterDeployment(withUnreachableCondition(corev1.ConditionFalse, time.Now().Add(-maxUnreachableDuration))),
			errorConnecting: pointer.BoolPtr(true),
			expectedStatus:  corev1.ConditionTrue,
			expectRequeue:   true,
		},
		{
			name:            "unreachable with unreachable condition",
			cd:              buildClusterDeployment(withUnreachableCondition(corev1.ConditionTrue, time.Now())),
			errorConnecting: pointer.BoolPtr(true),
			expectedStatus:  corev1.ConditionTrue,
			expectRequeue:   true,
		},
		{
			name:               "reachable with no condition",
			cd:                 buildClusterDeployment(),
			errorConnecting:    pointer.BoolPtr(false),
			expectedStatus:     corev1.ConditionFalse,
			expectRequeueAfter: true,
		},
		{
			name:               "reachable with old reachable condition",
			cd:                 buildClusterDeployment(withUnreachableCondition(corev1.ConditionFalse, time.Now().Add(-maxUnreachableDuration))),
			errorConnecting:    pointer.BoolPtr(false),
			expectedStatus:     corev1.ConditionFalse,
			expectRequeueAfter: true,
		},
		{
			name:               "reachable with unreachable condition",
			cd:                 buildClusterDeployment(withUnreachableCondition(corev1.ConditionTrue, time.Now())),
			errorConnecting:    pointer.BoolPtr(false),
			expectedStatus:     corev1.ConditionFalse,
			expectRequeueAfter: true,
		},
		{
			name:                          "reachable to primary with no conditions",
			cd:                            buildClusterDeployment(withAPIURLOverride()),
			errorConnecting:               pointer.BoolPtr(false),
			expectedStatus:                corev1.ConditionFalse,
			expectActiveOverrideCondition: true,
			expectedActiveOverrideStatus:  corev1.ConditionTrue,
			expectRequeueAfter:            true,
		},
		{
			name: "reachable to primary with recent reachable condition",
			cd: buildClusterDeployment(
				withAPIURLOverride(),
				withUnreachableCondition(corev1.ConditionFalse, time.Now()),
			),
			errorConnecting:               pointer.BoolPtr(false),
			expectedStatus:                corev1.ConditionFalse,
			expectActiveOverrideCondition: true,
			expectedActiveOverrideStatus:  corev1.ConditionTrue,
			expectRequeueAfter:            true,
		},
		{
			name:                          "reachable to secondary with no conditions",
			cd:                            buildClusterDeployment(withAPIURLOverride()),
			errorConnecting:               pointer.BoolPtr(true),
			errorConnectingSecondary:      pointer.BoolPtr(false),
			expectedStatus:                corev1.ConditionFalse,
			expectActiveOverrideCondition: true,
			expectedActiveOverrideStatus:  corev1.ConditionFalse,
			expectRequeue:                 true,
		},
		{
			name: "reachable to secondary with recent reachable to secondary",
			cd: buildClusterDeployment(
				withAPIURLOverride(),
				withUnreachableCondition(corev1.ConditionFalse, time.Now()),
				withActiveAPIURLOverrideCondition(corev1.ConditionFalse),
			),
			errorConnecting:               pointer.BoolPtr(true),
			expectedStatus:                corev1.ConditionFalse,
			expectActiveOverrideCondition: true,
			expectedActiveOverrideStatus:  corev1.ConditionFalse,
			expectRequeue:                 true,
		},
		{
			name: "reachable to secondary with old reachable to secondary",
			cd: buildClusterDeployment(
				withAPIURLOverride(),
				withUnreachableCondition(corev1.ConditionFalse, time.Now().Add(-maxUnreachableDuration)),
				withActiveAPIURLOverrideCondition(corev1.ConditionFalse),
			),
			errorConnecting:               pointer.BoolPtr(true),
			errorConnectingSecondary:      pointer.BoolPtr(false),
			expectedStatus:                corev1.ConditionFalse,
			expectActiveOverrideCondition: true,
			expectedActiveOverrideStatus:  corev1.ConditionFalse,
			expectRequeue:                 true,
		},
		{
			name: "reachable to primary with reachable to secondary condition",
			cd: buildClusterDeployment(
				withAPIURLOverride(),
				withUnreachableCondition(corev1.ConditionFalse, time.Now()),
				withActiveAPIURLOverrideCondition(corev1.ConditionFalse),
			),
			errorConnecting:               pointer.BoolPtr(false),
			expectedStatus:                corev1.ConditionFalse,
			expectActiveOverrideCondition: true,
			expectedActiveOverrideStatus:  corev1.ConditionTrue,
			expectRequeueAfter:            true,
		},
		{
			name: "reachable to secondary with old reachable to primary condition",
			cd: buildClusterDeployment(
				withAPIURLOverride(),
				withUnreachableCondition(corev1.ConditionFalse, time.Now().Add(-maxUnreachableDuration)),
				withActiveAPIURLOverrideCondition(corev1.ConditionTrue),
			),
			errorConnecting:               pointer.BoolPtr(true),
			errorConnectingSecondary:      pointer.BoolPtr(false),
			expectedStatus:                corev1.ConditionFalse,
			expectActiveOverrideCondition: true,
			expectedActiveOverrideStatus:  corev1.ConditionFalse,
			expectRequeue:                 true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			hivev1.AddToScheme(scheme)
			fakeClient := fake.NewFakeClientWithScheme(scheme, test.cd)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			if test.errorConnecting != nil {
				mockRemoteClientBuilder.EXPECT().UsePrimaryAPIURL().Return(mockRemoteClientBuilder)
				var buildError error
				if *test.errorConnecting {
					buildError = errors.New("cluster not reachable")
				}
				mockRemoteClientBuilder.EXPECT().Build().Return(nil, buildError)
			}
			if test.errorConnectingSecondary != nil {
				mockRemoteClientBuilder.EXPECT().UseSecondaryAPIURL().Return(mockRemoteClientBuilder)
				var buildError error
				if *test.errorConnectingSecondary {
					buildError = errors.New("cluster not reachable")
				}
				mockRemoteClientBuilder.EXPECT().Build().Return(nil, buildError)
			}
			rcd := &ReconcileRemoteMachineSet{
				Client:                        fakeClient,
				scheme:                        scheme,
				logger:                        log.WithField("controller", "unreachable"),
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
			}

			namespacedName := types.NamespacedName{
				Name:      testName,
				Namespace: testNamespace,
			}

			result, err := rcd.Reconcile(context.TODO(), reconcile.Request{NamespacedName: namespacedName})
			assert.NoError(t, err, "unexpected error during reconcile")

			cd := &hivev1.ClusterDeployment{}
			if err := fakeClient.Get(context.TODO(), namespacedName, cd); assert.NoError(t, err, "missing clusterdeployment") {
				cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
				if assert.NotNil(t, cond, "missing unreachable condition") {
					assert.Equal(t, string(test.expectedStatus), string(cond.Status), "unexpected status on unreachable condition")
				}
				cond = controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.ActiveAPIURLOverrideCondition)
				if !test.expectActiveOverrideCondition {
					assert.Nil(t, cond, "expected no active override condition")
				} else {
					if assert.NotNil(t, cond, "missing active override condition") {
						assert.Equal(t, string(test.expectedActiveOverrideStatus), string(cond.Status), "unexpected status on active override condition")
					}
				}
			}

			assert.Equal(t, test.expectRequeue, result.Requeue, "unexpected requeue")
			if test.expectRequeueAfter {
				assert.NotZero(t, result.RequeueAfter, "expected non-zero requeue after")
			} else {
				assert.Zero(t, result.RequeueAfter, "expected zero requeue after")
			}
		})
	}
}

func buildClusterDeployment(options ...testcd.Option) *hivev1.ClusterDeployment {
	options = append(
		[]testcd.Option{
			func(cd *hivev1.ClusterDeployment) {
				cd.Name = testName
				cd.Namespace = testNamespace
				cd.Spec.Installed = true
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{}
			},
		},
		options...,
	)
	return testcd.Build(options...)
}

func withUnreachableCondition(status corev1.ConditionStatus, probeTime time.Time) testcd.Option {
	return testcd.WithCondition(
		hivev1.ClusterDeploymentCondition{
			Type:          hivev1.UnreachableCondition,
			Status:        status,
			LastProbeTime: metav1.NewTime(probeTime),
		},
	)
}

func withActiveAPIURLOverrideCondition(status corev1.ConditionStatus) testcd.Option {
	return testcd.WithCondition(
		hivev1.ClusterDeploymentCondition{
			Type:   hivev1.ActiveAPIURLOverrideCondition,
			Status: status,
		},
	)
}

func withAPIURLOverride() testcd.Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.ControlPlaneConfig.APIURLOverride = "some-api-url"
	}
}
