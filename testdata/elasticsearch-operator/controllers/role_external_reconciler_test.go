package controllers

import (
	"testing"

	eshandler "github.com/disaster37/es-handler/v8"
	elasticsearchapicrd "github.com/disaster37/operator-sdk-extra/v2/testdata/elasticsearch-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRoleBuild(t *testing.T) {

	var (
		o            *elasticsearchapicrd.Role
		role         *eshandler.XPackSecurityRole
		expectedRole *eshandler.XPackSecurityRole
		err          error
	)

	client := &roleApiClient{}

	// Normal case
	o = &elasticsearchapicrd.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: elasticsearchapicrd.RoleSpec{
			ElasticsearchRef: elasticsearchapicrd.ElasticsearchRef{
				ExternalElasticsearchRef: &elasticsearchapicrd.ElasticsearchExternalRef{
					Addresses: []string{
						"https://es.default.svc:9200",
					},
				},
			},
			Cluster: []string{
				"monitor",
			},
			Indices: []elasticsearchapicrd.RoleSpecIndicesPermissions{
				{
					Names: []string{
						"*",
					},
					Privileges: []string{
						"view_index_metadata",
						"monitor",
					},
				},
			},
		},
	}

	expectedRole = &eshandler.XPackSecurityRole{
		Cluster: []string{
			"monitor",
		},
		Indices: []eshandler.XPackSecurityIndicesPermissions{
			{
				Names: []string{
					"*",
				},
				Privileges: []string{
					"view_index_metadata",
					"monitor",
				},
			},
		},
	}

	role, err = client.Build(o)
	assert.NoError(t, err)
	assert.Equal(t, expectedRole, role)

	// With all parameters
	o = &elasticsearchapicrd.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: elasticsearchapicrd.RoleSpec{
			ElasticsearchRef: elasticsearchapicrd.ElasticsearchRef{
				ExternalElasticsearchRef: &elasticsearchapicrd.ElasticsearchExternalRef{
					Addresses: []string{
						"https://es.default.svc:9200",
					},
				},
			},
			Cluster: []string{
				"monitor",
			},
			Indices: []elasticsearchapicrd.RoleSpecIndicesPermissions{
				{
					Names: []string{
						"*",
					},
					Privileges: []string{
						"view_index_metadata",
						"monitor",
					},
					AllowRestrictedIndices: true,
					FieldSecurity: `
{
	"grant" : [ "title", "body" ]
}
					`,
					Query: `
{
	"match": {
		"title": "foo"
	}
}`,
				},
			},
			Applications: []elasticsearchapicrd.RoleSpecApplicationPrivileges{
				{
					Application: "myapp",
					Privileges: []string{
						"admin",
						"read",
					},
					Resources: []string{
						"*",
					},
				},
			},
			RunAs: []string{
				"other_user",
			},
			Metadata: `
{
	"version" : 1
}
			`,
			TransientMetadata: `
{
	"key": "value"
}
			`,
		},
	}

	expectedRole = &eshandler.XPackSecurityRole{
		Cluster: []string{
			"monitor",
		},
		Indices: []eshandler.XPackSecurityIndicesPermissions{
			{
				Names: []string{
					"*",
				},
				Privileges: []string{
					"view_index_metadata",
					"monitor",
				},
				AllowRestrictedIndices: true,
				FieldSecurity: map[string]any{
					"grant": []any{
						"title",
						"body",
					},
				},
				Query: `
{
	"match": {
		"title": "foo"
	}
}`,
			},
		},
		Applications: []eshandler.XPackSecurityApplicationPrivileges{
			{
				Application: "myapp",
				Privileges: []string{
					"admin",
					"read",
				},
				Resources: []string{
					"*",
				},
			},
		},
		RunAs: []string{
			"other_user",
		},
		Metadata: map[string]any{
			"version": float64(1),
		},
		TransientMetadata: map[string]any{
			"key": "value",
		},
	}

	role, err = client.Build(o)
	assert.NoError(t, err)
	assert.Equal(t, expectedRole, role)

}
