package venafi

import (
	"fmt"

	"github.com/Venafi/vcert"
	"github.com/Venafi/vcert/pkg/endpoint"
	cmapi "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1alpha1"
	"github.com/jetstack/cert-manager/pkg/controller"
	"github.com/jetstack/cert-manager/pkg/issuer"
	corev1 "k8s.io/api/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
)

const (
	tppUsernameKey = "username"
	tppPasswordKey = "password"

	defaultAPIKeyKey = "api-key"
)

// Veanfi is a implementation of govcert library to manager certificates from TPP or Venafi Cloud

type Venafi struct {
	issuer cmapi.GenericIssuer
	*controller.Context

	// Namespace in which to read resources related to this Issuer from.
	// For Issuers, this will be the namespace of the Issuer.
	// For ClusterIssuers, this will be the cluster resource namespace.
	resourceNamespace string
	secretsLister     corelisters.SecretLister

	client endpoint.Connector
}

func NewVenafi(ctx *controller.Context, issuer cmapi.GenericIssuer) (issuer.Interface, error) {
	secretsLister := ctx.KubeSharedInformerFactory.Core().V1().Secrets().Lister()
	resourceNamespace := ctx.IssuerOptions.ResourceNamespace(issuer)

	cfg, err := configForIssuer(issuer, secretsLister, resourceNamespace)
	if err != nil {
		ctx.Recorder.Eventf(issuer, corev1.EventTypeWarning, "FailedInit", "Failed to initialise issuer: %v", err)
		return nil, err
	}

	client, err := vcert.NewClient(cfg)
	if err != nil {
		ctx.Recorder.Eventf(issuer, corev1.EventTypeWarning, "FailedInit", "Failed to create Venafi client: %v", err)
		return nil, fmt.Errorf("error creating Venafi client: %s", err.Error())
	}

	return &Venafi{
		issuer:            issuer,
		Context:           ctx,
		resourceNamespace: resourceNamespace,
		secretsLister:     secretsLister,
		client:            client,
	}, nil
}

// configForIssuer will convert a cert-manager Venafi issuer into a vcert.Config
// that can be used to instantiate an API client.
func configForIssuer(iss cmapi.GenericIssuer, secretsLister corelisters.SecretLister, resourceNamespace string) (*vcert.Config, error) {
	venCfg := iss.GetSpec().Venafi
	switch {
	case venCfg.TPP != nil:
		tpp := venCfg.TPP
		tppSecret, err := secretsLister.Secrets(resourceNamespace).Get(tpp.CredentialsRef.Name)
		if err != nil {
			return nil, fmt.Errorf("error loading TPP credentials: %v", err)
		}

		username := tppSecret.Data[tppUsernameKey]
		password := tppSecret.Data[tppPasswordKey]

		caBundle := ""
		if len(tpp.CABundle) > 0 {
			caBundle = string(tpp.CABundle)
		}

		return &vcert.Config{
			ConnectorType:   endpoint.ConnectorTypeTPP,
			BaseUrl:         tpp.URL,
			Zone:            venCfg.Zone,
			LogVerbose:      venCfg.Verbose,
			ConnectionTrust: caBundle,
			Credentials: &endpoint.Authentication{
				User:     string(username),
				Password: string(password),
			},
		}, nil

	case venCfg.Cloud != nil:
		cloud := venCfg.Cloud
		cloudSecret, err := secretsLister.Secrets(resourceNamespace).Get(cloud.APIKeySecretRef.Name)
		if err != nil {
			return nil, fmt.Errorf("error loading TPP credentials: %v", err)
		}

		k := defaultAPIKeyKey
		if cloud.APIKeySecretRef.Key != "" {
			k = cloud.APIKeySecretRef.Key
		}
		apiKey := cloudSecret.Data[k]

		return &vcert.Config{
			ConnectorType: endpoint.ConnectorTypeCloud,
			BaseUrl:       cloud.URL,
			Zone:          venCfg.Zone,
			LogVerbose:    venCfg.Verbose,
			Credentials: &endpoint.Authentication{
				APIKey: string(apiKey),
			},
		}, nil

	default:
		return nil, fmt.Errorf("neither Venafi Cloud or TPP configuration found")
	}
}

func init() {
	controller.RegisterIssuer(controller.IssuerVenafi, NewVenafi)
}
