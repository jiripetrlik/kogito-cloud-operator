// Copyright 2020 Red Hat, Inc. and/or its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kogitoinfra

import (
	"fmt"
	"net/url"

	"github.com/kiegroup/kogito-cloud-operator/pkg/apis/app/v1beta1"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client/kubernetes"
	"github.com/kiegroup/kogito-cloud-operator/pkg/framework"
	"github.com/kiegroup/kogito-cloud-operator/pkg/infrastructure"
	mongodb "github.com/mongodb/mongodb-kubernetes-operator/pkg/apis/mongodb/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	mongoDBSecretName = "kogito-mongodb-credential"

	// Using URI for Quarkus as this is what we get from MongoDB instance
	// and host/port for Spring Boot because URI cannot be used with credentials (Spring Boot Starter restriction) ...
	// https://github.com/spring-projects/spring-boot/blob/b7fdf8fe87da1c01ff6aca041170a02f11280a1a/spring-boot-project/spring-boot-autoconfigure/src/main/java/org/springframework/boot/autoconfigure/mongo/MongoProperties.java#L61-L64
	appPropMongoDBURI  = iota // for Quarkus
	appPropMongoDBHost        // for Spring boot
	appPropMongoDBPort        // for Spring boot

	envVarMongoDBAuthDatabase
	envVarMongoDBUser
	envVarMongoDBPassword
	envVarMongoDBDatabase

	mongoDBEnvKeyCredSecret        = "MONGODB_CREDENTIAL_SECRET"
	mongoDBEnablePersistenceEnvKey = "ENABLE_PERSISTENCE"

	infraPropertiesUserKey         = "username"
	infraPropertiesDatabaseKey     = "database"
	infraPropertiesAuthDatabaseKey = "auth-database"
)

var (
	// MongoDB variables for the KogitoInfra deployed infrastructure.
	//For Quarkus: https://quarkus.io/guides/mongoDB-client#quarkus-mongoDB-client_configuration
	//For Spring: https://github.com/mongoDB/mongoDB-spring-boot/blob/master/mongoDB-spring-boot-starter-remote/src/test/resources/test-application.properties

	// propertiesMongoDBQuarkus mongoDB properties for quarkus runtime
	propertiesMongoDBQuarkus = map[int]string{
		appPropMongoDBURI: "quarkus.mongodb.connection-string",

		envVarMongoDBAuthDatabase: "QUARKUS_MONGODB_CREDENTIALS_AUTH_SOURCE",
		envVarMongoDBUser:         "QUARKUS_MONGODB_CREDENTIALS_USERNAME",
		envVarMongoDBPassword:     "QUARKUS_MONGODB_CREDENTIALS_PASSWORD",
		envVarMongoDBDatabase:     "QUARKUS_MONGODB_DATABASE",
	}
	// propertiesMongoDBSpring mongoDB properties for spring boot runtime
	propertiesMongoDBSpring = map[int]string{
		appPropMongoDBHost: "spring.data.mongodb.host",
		appPropMongoDBPort: "spring.data.mongodb.port",

		envVarMongoDBAuthDatabase: "SPRING_DATA_MONGODB_AUTHENTICATION_DATABASE",
		envVarMongoDBUser:         "SPRING_DATA_MONGODB_USERNAME",
		envVarMongoDBPassword:     "SPRING_DATA_MONGODB_PASSWORD",
		envVarMongoDBDatabase:     "SPRING_DATA_MONGODB_DATABASE",
	}
)

func (i *mongoDBInfraReconciler) getMongoDBSecretEnvVars(mongoDBInstance *mongodb.MongoDB) ([]corev1.EnvVar, error) {
	var envProps []corev1.EnvVar

	customMongoDBSecret, resultErr := i.loadCustomKogitoMongoDBSecret(i.instance.Namespace)
	if resultErr != nil {
		return nil, resultErr
	}

	if customMongoDBSecret == nil {
		customMongoDBSecret, resultErr = i.createCustomKogitoMongoDBSecret(i.instance.Namespace, mongoDBInstance)
		if resultErr != nil {
			return nil, resultErr
		}
	}

	envProps = append(envProps, framework.CreateEnvVar(mongoDBEnablePersistenceEnvKey, "true"))
	mongoDBSecretName := customMongoDBSecret.Name
	envProps = append(envProps, framework.CreateEnvVar(mongoDBEnvKeyCredSecret, mongoDBSecretName))
	envProps = append(envProps, framework.CreateSecretEnvVar(propertiesMongoDBSpring[envVarMongoDBAuthDatabase], mongoDBSecretName, infrastructure.MongoDBAppSecretAuthDatabaseKey))
	envProps = append(envProps, framework.CreateSecretEnvVar(propertiesMongoDBQuarkus[envVarMongoDBAuthDatabase], mongoDBSecretName, infrastructure.MongoDBAppSecretAuthDatabaseKey))
	envProps = append(envProps, framework.CreateSecretEnvVar(propertiesMongoDBSpring[envVarMongoDBUser], mongoDBSecretName, infrastructure.MongoDBAppSecretUsernameKey))
	envProps = append(envProps, framework.CreateSecretEnvVar(propertiesMongoDBQuarkus[envVarMongoDBUser], mongoDBSecretName, infrastructure.MongoDBAppSecretUsernameKey))
	envProps = append(envProps, framework.CreateSecretEnvVar(propertiesMongoDBSpring[envVarMongoDBPassword], mongoDBSecretName, infrastructure.MongoDBAppSecretPasswordKey))
	envProps = append(envProps, framework.CreateSecretEnvVar(propertiesMongoDBQuarkus[envVarMongoDBPassword], mongoDBSecretName, infrastructure.MongoDBAppSecretPasswordKey))
	envProps = append(envProps, framework.CreateSecretEnvVar(propertiesMongoDBSpring[envVarMongoDBDatabase], mongoDBSecretName, infrastructure.MongoDBAppSecretDatabaseKey))
	envProps = append(envProps, framework.CreateSecretEnvVar(propertiesMongoDBQuarkus[envVarMongoDBDatabase], mongoDBSecretName, infrastructure.MongoDBAppSecretDatabaseKey))
	return envProps, nil
}

func (i *mongoDBInfraReconciler) getMongoDBAppProps(mongoDBInstance *mongodb.MongoDB) (map[string]string, error) {
	appProps := map[string]string{}

	mongoDBURI := mongoDBInstance.Status.MongoURI
	if len(mongoDBURI) > 0 {
		mongoDBParsedURL, err := url.ParseRequestURI(mongoDBURI)
		if err != nil {
			return nil, err
		}
		appProps[propertiesMongoDBSpring[appPropMongoDBHost]] = mongoDBParsedURL.Hostname()
		appProps[propertiesMongoDBSpring[appPropMongoDBPort]] = mongoDBParsedURL.Port()
		appProps[propertiesMongoDBQuarkus[appPropMongoDBURI]] = mongoDBURI
	}

	return appProps, nil
}

func (i *mongoDBInfraReconciler) updateMongoDBAppPropsInStatus(mongoDBInstance *mongodb.MongoDB) error {
	log.Debug("going to Update MongoDB app properties in kogito infra instance status")
	appProps, err := i.getMongoDBAppProps(mongoDBInstance)
	if err != nil {
		return err
	}
	i.instance.Status.AppProps = appProps
	log.Debugf("Following app properties are set infra status : %s", appProps)
	return nil
}

func (i *mongoDBInfraReconciler) updateMongoDBEnvVarsInStatus(mongoDBInstance *mongodb.MongoDB) error {
	log.Debug("going to Update MongoDB env properties in kogito infra instance status")
	envVars, err := i.getMongoDBSecretEnvVars(mongoDBInstance)
	if err != nil {
		return err
	}
	i.instance.Status.Env = envVars
	log.Debugf("Following env properties are set infra status : %s", envVars)
	return nil
}

func (i *mongoDBInfraReconciler) loadDeployedMongoDBInstance(instanceName string, namespace string) (*mongodb.MongoDB, error) {
	log.Debug("fetching deployed kogito mongoDB instance")
	mongoDBInstance := &mongodb.MongoDB{}
	if exists, err := kubernetes.ResourceC(i.client).FetchWithKey(types.NamespacedName{Name: instanceName, Namespace: namespace}, mongoDBInstance); err != nil {
		log.Error("Error occurs while fetching kogito mongoDB instance")
		return nil, err
	} else if !exists {
		log.Debug("Kogito mongoDB instance is not exists")
		return nil, nil
	} else {
		log.Debug("Kogito mongoDB instance found")
		log.Debug("Kogito mongoDB instance found %v", mongoDBInstance)
		return mongoDBInstance, nil
	}
}

func (i *mongoDBInfraReconciler) loadCustomKogitoMongoDBSecret(namespace string) (*corev1.Secret, error) {
	log.Debugf("Fetching %s ", mongoDBSecretName)
	secret := &corev1.Secret{}
	if exists, err := kubernetes.ResourceC(i.client).FetchWithKey(types.NamespacedName{Name: mongoDBSecretName, Namespace: namespace}, secret); err != nil {
		log.Errorf("Error occurs while fetching %s", mongoDBSecretName)
		return nil, err
	} else if !exists {
		log.Debugf("%s not found", mongoDBSecretName)
		return nil, nil
	} else {
		log.Debugf("%s successfully fetched", mongoDBSecretName)
		return secret, nil
	}
}

// Setup authentication to MongoDB
// https://github.com/mongodb/mongodb-kubernetes-operator/blob/master/docs/users.md
func (i *mongoDBInfraReconciler) createCustomKogitoMongoDBSecret(namespace string, mongoDBInstance *mongodb.MongoDB) (*corev1.Secret, error) {
	log.Debugf("Creating new secret %s", mongoDBSecretName)

	credentials, err := retrieveMongoDBCredentialsFromInstance(i.client, i.instance, mongoDBInstance)
	if err != nil {
		return nil, err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mongoDBSecretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			infrastructure.MongoDBAppSecretAuthDatabaseKey: credentials.AuthDatabase,
			infrastructure.MongoDBAppSecretUsernameKey:     credentials.Username,
			infrastructure.MongoDBAppSecretPasswordKey:     credentials.Password,
			infrastructure.MongoDBAppSecretDatabaseKey:     credentials.Database,
		},
	}
	if err := framework.SetOwner(i.instance, i.scheme, secret); err != nil {
		return nil, err
	}
	if err := kubernetes.ResourceC(i.client).Create(secret); err != nil {
		log.Errorf("Error occurs while creating %s", secret)
		return nil, err
	}
	log.Debug("%s successfully created", secret)
	return secret, nil
}

type mongoDBInfraReconciler struct {
	targetContext
}

// getMongoDBWatchedObjects provide list of object that needs to be watched to maintain MongoDB kogitoInfra resource
func getMongoDBWatchedObjects() []framework.WatchedObjects {
	return []framework.WatchedObjects{
		{
			GroupVersion: mongodb.SchemeGroupVersion,
			AddToScheme:  mongodb.SchemeBuilder.AddToScheme,
			Objects:      []runtime.Object{&mongodb.MongoDB{}},
		},
		{
			Objects: []runtime.Object{&corev1.Secret{}},
		},
	}
}

// Reconcile reconcile Kogito infra object
func (i *mongoDBInfraReconciler) Reconcile() (requeue bool, resultErr error) {
	var mongoDBInstance *mongodb.MongoDB

	if !infrastructure.IsMongoDBAvailable(i.client) {
		return false, errorForResourceAPINotFound(&i.instance.Spec.Resource)
	}

	// Step 1: check whether user has provided custom mongoDB instance reference
	mongoDBNamespace := i.instance.Spec.Resource.Namespace
	mongoDBName := i.instance.Spec.Resource.Name
	if len(mongoDBNamespace) == 0 {
		mongoDBNamespace = i.instance.Namespace
		log.Debugf("Namespace is not provided for infrastructure MongoDB resource %s, taking instance namespace(%s) as default", i.instance.Name, mongoDBNamespace)
	}
	if len(mongoDBName) == 0 {
		return false, errorForResourceConfigError(i.instance, "No resource name given")
	}

	if mongoDBInstance, resultErr = i.loadDeployedMongoDBInstance(mongoDBName, mongoDBNamespace); resultErr != nil {
		return false, resultErr
	} else if mongoDBInstance == nil {
		return false, errorForResourceNotFound("MongoDB", i.instance.Spec.Resource.Name, mongoDBNamespace)
	}

	log.Debugf("Got MongoDB instance %v", mongoDBInstance)
	if mongoDBInstance.Status.Phase != mongodb.Running {
		return false, errorForResourceNotReadyError(fmt.Errorf("mongoDB instance %s not ready. Waiting for Status.Phase == Running", mongoDBInstance.Name))
	}
	log.Info("MongoDB instance is running")
	if resultErr = i.updateMongoDBAppPropsInStatus(mongoDBInstance); resultErr != nil {
		return true, resultErr
	}
	if resultErr = i.updateMongoDBEnvVarsInStatus(mongoDBInstance); resultErr != nil {
		return true, resultErr
	}
	return false, resultErr
}

// MongoDBCredential holds the credentials information of a user into an mongoDB server
type MongoDBCredential struct {
	AuthDatabase string `yaml:"auth-database"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	Database     string `yaml:"database"`
}

// retrieveMongoDBCredentialsFromInstance retrieves the credentials of the MongoDB server deployed with the Kogito Operator
// based on the kogitoinfra given properties
func retrieveMongoDBCredentialsFromInstance(cli *client.Client, kogitoInfra *v1beta1.KogitoInfra, mongoDBInstance *mongodb.MongoDB) (*MongoDBCredential, error) {
	creds := &MongoDBCredential{}
	if len(kogitoInfra.Spec.InfraProperties[infraPropertiesUserKey]) == 0 {
		return nil, errorForMissingResourceConfig(kogitoInfra, infraPropertiesUserKey)
	} else if len(kogitoInfra.Spec.InfraProperties[infraPropertiesDatabaseKey]) == 0 {
		return nil, errorForMissingResourceConfig(kogitoInfra, infraPropertiesDatabaseKey)
	}
	creds.Username = kogitoInfra.Spec.InfraProperties[infraPropertiesUserKey]
	creds.Database = kogitoInfra.Spec.InfraProperties[infraPropertiesDatabaseKey]
	creds.AuthDatabase = kogitoInfra.Spec.InfraProperties[infraPropertiesAuthDatabaseKey]
	if len(creds.AuthDatabase) == 0 {
		creds.AuthDatabase = infrastructure.DefaultMongoDBAuthDatabase
	}

	user := findMongoDBUserByUsernameAndAuthDatabase(mongoDBInstance, creds.Username, creds.AuthDatabase)
	if user == nil {
		return nil, errorForResourceConfigError(kogitoInfra, fmt.Sprintf("No user found in MongoDB configuration for username %s and authentication database %s", creds.Username, creds.AuthDatabase))
	}
	log.Debugf("Found user %s with authDB %s and password ref %s", user.Name, user.DB, user.PasswordSecretRef.Name)

	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: user.PasswordSecretRef.Name, Namespace: mongoDBInstance.Namespace}}
	if exists, err := kubernetes.ResourceC(cli).Fetch(secret); err != nil {
		return nil, err
	} else if !exists {
		return nil, errorForResourceNotFound("Secret", user.PasswordSecretRef.Name, kogitoInfra.Namespace)
	} else {
		log.Debugf("Found MongoDB secret for password ref %s", user.PasswordSecretRef.Name)
		passwordKey := infrastructure.DefaultMongoDBPasswordSecretRef
		if user.PasswordSecretRef.Key != "" {
			passwordKey = user.PasswordSecretRef.Key
		}
		creds.Password = string(secret.Data[passwordKey])
	}

	return creds, nil
}

func findMongoDBUserByUsernameAndAuthDatabase(mongoDBInstance *mongodb.MongoDB, username, authDB string) *mongodb.MongoDBUser {
	log.Debugf("Looking for user %s in authDB %s", username, authDB)

	for _, user := range mongoDBInstance.Spec.Users {
		if user.DB == authDB {
			if user.Name == username {
				return &user
			}
		}
	}

	return nil
}
