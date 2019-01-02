/*

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

package v1alpha1

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&RandomParagraphApp{}, &RandomParagraphAppList{})
}

// RandomParagraphAppSpec defines the desired state of RandomParagraphApp
type RandomParagraphAppSpec struct {
	// Version specifies the version of the service to create
	//+kubebuilder:validation:MinLength=1
	Version string `json:"version,omitempty"`
	// Replicas specifies how many instances of the service we want
	//+kubebuilder:validation:Minimum=0
	Replicas int `json:"replicas,omitempty"`
}

// RandomParagraphAppStatus defines the observed state of RandomParagraphApp
type RandomParagraphAppStatus struct {
	// Conditions keeps a record of the last 20 application conditions
	Conditions []ApplicationCondition `json:"conditions"`
	// Replicas is the current number of replicas
	//+kubebuilder:validation:Minimum=0
	Replicas int `json:"replicas"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RandomParagraphApp is the Schema for the randomparagraphapps API
// +k8s:openapi-gen=true
type RandomParagraphApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RandomParagraphAppSpec   `json:"spec,omitempty"`
	Status RandomParagraphAppStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RandomParagraphAppList contains a list of RandomParagraphApp
type RandomParagraphAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RandomParagraphApp `json:"items"`
}

type ApplicationCondition struct {
	Type   ApplicationConditionType `json:"type"`
	Reason string                   `json:"reason"`
	Time   string                   `json:"time"`
}

type ApplicationConditionType string

const (
	ApplicationConditionReady     = "Ready"
	ApplicationConditionScale     = "Scale"
	ApplicationConditionUpgrading = "Upgrading"
)

func (rs *RandomParagraphAppStatus) SetScaleCondition(from, to int) {
	c := ApplicationCondition{
		Type:   ApplicationConditionScale,
		Reason: fmt.Sprintf("scaled pods from %d to %d", from, to),
		Time:   time.Now().Format(time.RFC3339),
	}
	rs.appendCondition(c)
}

func (rs *RandomParagraphAppStatus) SetReadyCondition() {
	c := ApplicationCondition{
		Type:   ApplicationConditionReady,
		Reason: "current state matches desired state",
		Time:   time.Now().Format(time.RFC3339),
	}

	if len(rs.Conditions) == 0 {
		rs.appendCondition(c)
		return
	}

	last := rs.Conditions[len(rs.Conditions)-1]
	if last.Type == ApplicationConditionReady {
		// Do nothing if the last status is already Ready
		return
	}

	rs.appendCondition(c)
}

func (rs *RandomParagraphAppStatus) SetUpgradedCondition(numberUpdated int, version string) {
	c := ApplicationCondition{
		Type:   ApplicationConditionUpgrading,
		Reason: fmt.Sprintf("upgraded %d pods to %s", numberUpdated, version),
		Time:   time.Now().Format(time.RFC3339),
	}
	rs.appendCondition(c)
}

func (rs *RandomParagraphAppStatus) appendCondition(c ApplicationCondition) {
	rs.Conditions = append(rs.Conditions, c)
	if len(rs.Conditions) > 20 {
		rs.Conditions = rs.Conditions[1:]
	}
}
