/*
 * Nudm_SDM
 *
 * Nudm Subscriber Data Management Service
 *
 * API version: 2.0.0
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package models

type SubscriptionDataSets struct {
	AmData      *AccessAndMobilitySubscriptionData  `json:"amData,omitempty" yaml:"amData" bson:"amData" mapstructure:"AmData"`
	SmfSelData  *SmfSelectionSubscriptionData       `json:"smfSelData,omitempty" yaml:"smfSelData" bson:"smfSelData" mapstructure:"SmfSelData"`
	UecSmfData  *UeContextInSmfData                 `json:"uecSmfData,omitempty" yaml:"uecSmfData" bson:"uecSmfData" mapstructure:"UecSmfData"`
	UecSmsfData *UeContextInSmsfData                `json:"uecSmsfData,omitempty" yaml:"uecSmsfData" bson:"uecSmsfData" mapstructure:"UecSmsfData"`
	SmsSubsData *SmsSubscriptionData                `json:"smsSubsData,omitempty" yaml:"smsSubsData" bson:"smsSubsData" mapstructure:"SmsSubsData"`
	SmData      []SessionManagementSubscriptionData `json:"smData,omitempty" yaml:"smData" bson:"smData" mapstructure:"SmData"`
	TraceData   *TraceData                          `json:"traceData,omitempty" yaml:"traceData" bson:"traceData" mapstructure:"TraceData"`
	SmsMngData  *SmsManagementSubscriptionData      `json:"smsMngData,omitempty" yaml:"smsMngData" bson:"smsMngData" mapstructure:"SmsMngData"`
}
