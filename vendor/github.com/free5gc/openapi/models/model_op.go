/*
 * Nudr_DataRepository API OpenAPI file
 *
 * Unified Data Repository Service
 *
 * API version: 1.0.0
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package models

type Op struct {
	OpValue             string `json:"opValue" bson:"opValue"`
	EncryptionKey       int32  `json:"encryptionKey" bson:"encryptionKey"`
	EncryptionAlgorithm int32  `json:"encryptionAlgorithm" bson:"encryptionAlgorithm"`
}
