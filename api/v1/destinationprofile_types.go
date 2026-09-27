// =============================================================================
// Bu dosya ne ise yarar?
//   DestinationProfile CRD Go tipi: paylasilan S3/MinIO hedef tanimi.
// =============================================================================

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DestinationProfileSpec is a reusable S3-compatible destination.
type DestinationProfileSpec struct {
	ObjectStorageSpec `json:",inline"`
}

// DestinationProfileStatus is reserved for future observations.
type DestinationProfileStatus struct {
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=bdp
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.spec.endpoint`
// +kubebuilder:printcolumn:name="Bucket",type=string,JSONPath=`.spec.bucket`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DestinationProfile names a shared object-storage destination (multi-cluster / multi-bucket profiles).
type DestinationProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DestinationProfileSpec   `json:"spec,omitempty"`
	Status DestinationProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DestinationProfileList contains DestinationProfile items.
type DestinationProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DestinationProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DestinationProfile{}, &DestinationProfileList{})
}
