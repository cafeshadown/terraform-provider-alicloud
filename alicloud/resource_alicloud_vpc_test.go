package alicloud

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaesslerAG/jsonpath"
	"github.com/agiledragon/gomonkey/v2"
	"github.com/alibabacloud-go/tea-rpc/client"
	util "github.com/alibabacloud-go/tea-utils/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/hashicorp/terraform-plugin-sdk/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
	"github.com/stretchr/testify/assert"

	"github.com/aliyun/terraform-provider-alicloud/alicloud/connectivity"
	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
)

func init() {
	resource.AddTestSweepers("alicloud_vpc", &resource.Sweeper{
		Name: "alicloud_vpc",
		F:    testSweepVpcs,
		// When implemented, these should be removed firstly
		Dependencies: []string{
			"alicloud_vswitch",
			"alicloud_nat_gateway",
			"alicloud_security_group",
			"alicloud_ots_instance",
			"alicloud_router_interface",
			"alicloud_route_table",
			"alicloud_cen_instance",
			"alicloud_edas_cluster",
			"alicloud_edas_k8s_cluster",
			"alicloud_network_acl",
			"alicloud_cs_kubernetes",
		},
	})
}

func testSweepVpcs(region string) error {
	rawClient, err := sharedClientForRegion(region)
	if err != nil {
		return fmt.Errorf("error getting Alicloud client: %s", err)
	}
	client := rawClient.(*connectivity.AliyunClient)

	prefixes := []string{
		"tf-testAcc",
		"tf_testAcc",
		"tf_test_",
		"tf-test-",
		"testAcc",
	}

	vpcIds := make([]string, 0)
	conn, err := client.NewVpcClient()
	if err != nil {
		return WrapError(err)
	}
	action := "DescribeVpcs"
	var response map[string]interface{}
	request := map[string]interface{}{
		"PageSize":   PageSizeLarge,
		"PageNumber": 1,
		"RegionId":   client.RegionId,
	}
	for {
		runtime := util.RuntimeOptions{}
		runtime.SetAutoretry(true)
		response, err = conn.DoRequest(StringPointer(action), nil, StringPointer("POST"), StringPointer("2016-04-28"), StringPointer("AK"), nil, request, &runtime)
		if err != nil {
			log.Printf("[ERROR] Failed to retrieve VPC in service list: %s", err)
			return nil
		}
		resp, err := jsonpath.Get("$.Vpcs.Vpc", response)
		if err != nil {
			return WrapErrorf(err, FailedGetAttributeMsg, action, "$.Vpcs.Vpc", response)
		}
		result, _ := resp.([]interface{})
		for _, v := range result {
			skip := true
			item := v.(map[string]interface{})
			// Skip the default vpc
			if v, ok := item["IsDefault"].(bool); ok && v {
				continue
			}
			for _, prefix := range prefixes {
				if strings.HasPrefix(strings.ToLower(fmt.Sprint(item["VpcName"])), strings.ToLower(prefix)) {
					skip = false
					break
				}
			}
			if skip {
				log.Printf("[INFO] Skipping VPC: %v (%v)", item["VpcName"], item["VpcId"])
				continue
			}
			vpcIds = append(vpcIds, fmt.Sprint(item["VpcId"]))
		}
		if len(result) < PageSizeLarge {
			break
		}
		request["PageNumber"] = request["PageNumber"].(int) + 1
	}

	for _, id := range vpcIds {
		log.Printf("[INFO] Deleting VPC: (%s)", id)
		action := "DeleteVpc"
		request := map[string]interface{}{
			"VpcId":    id,
			"RegionId": client.RegionId,
		}
		wait := incrementalWait(3*time.Second, 3*time.Second)
		err = resource.Retry(time.Minute*10, func() *resource.RetryError {
			response, err = conn.DoRequest(StringPointer(action), nil, StringPointer("POST"), StringPointer("2016-04-28"), StringPointer("AK"), nil, request, &util.RuntimeOptions{})
			if err != nil {
				if NeedRetry(err) {
					wait()
					return resource.RetryableError(err)
				}
				return resource.NonRetryableError(err)
			}
			return nil
		})
		if err != nil {
			log.Printf("[ERROR] Failed to delete VPC (%s): %v", id, err)
			continue
		}
	}
	return nil
}

func TestAccAlicloudVpc_basic(t *testing.T) {
	var v map[string]interface{}
	resourceId := "alicloud_vpc.default"
	ra := resourceAttrInit(resourceId, AlicloudVpcMap)
	rc := resourceCheckInitWithDescribeMethod(resourceId, &v, func() interface{} {
		return &VpcService{testAccProvider.Meta().(*connectivity.AliyunClient)}
	}, "DescribeVpc")
	rac := resourceAttrCheckInit(rc, ra)
	testAccCheck := rac.resourceAttrMapUpdateSet()
	rand := acctest.RandIntRange(1000000, 9999999)
	name := fmt.Sprintf("tf-testAcc%sVpc%d", defaultRegionToTest, rand)
	testAccConfig := resourceTestAccConfigFunc(resourceId, name, AlicloudVpcBasicDependence)
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckWithRegions(t, true, connectivity.VpcIpv6SupportRegions)
		},

		IDRefreshName: resourceId,
		Providers:     testAccProviders,
		CheckDestroy:  rac.checkResourceDestroy(),
		Steps: []resource.TestStep{
			{
				Config: testAccConfig(map[string]interface{}{
					"user_cidrs": []string{"106.11.62.0/24"},
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"user_cidrs.#": "1",
					}),
				),
			},
			{
				ResourceName:            resourceId,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"dry_run", "enable_ipv6"},
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"cidr_block": "172.16.0.0/16",
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"cidr_block": "172.16.0.0/16",
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"vpc_name": name,
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"vpc_name": name,
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"description": name,
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"description": name,
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"secondary_cidr_blocks": []string{"10.0.0.0/8"},
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"secondary_cidr_blocks.#": "1",
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"resource_group_id": "${data.alicloud_resource_manager_resource_groups.default.groups.0.id}",
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"resource_group_id": CHECKSET,
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"tags": map[string]string{
						"Created": "TF",
						"For":     "Test",
					},
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"tags.%":       "2",
						"tags.Created": "TF",
						"tags.For":     "Test",
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"tags": map[string]string{
						"Created": "TF-update",
						"For":     "Test-update",
					},
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"tags.%":       "2",
						"tags.Created": "TF-update",
						"tags.For":     "Test-update",
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"cidr_block":            "172.16.0.0/12",
					"vpc_name":              name + "update",
					"description":           name + "update",
					"secondary_cidr_blocks": []string{"10.0.0.0/8"},
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"cidr_block":              "172.16.0.0/12",
						"vpc_name":                name + "update",
						"description":             name + "update",
						"secondary_cidr_blocks.#": "1",
					}),
				),
			},
		},
	})
}

var AlicloudVpcMap = map[string]string{
	"status":          CHECKSET,
	"router_id":       CHECKSET,
	"router_table_id": CHECKSET,
	"route_table_id":  CHECKSET,
	"ipv6_cidr_block": "",
}

func AlicloudVpcBasicDependence(name string) string {
	return fmt.Sprintf(`

data "alicloud_resource_manager_resource_groups" "default" {
  name_regex = "default"
}
`)
}

func TestAccAlicloudVpc_enableIpv6(t *testing.T) {
	var v map[string]interface{}
	resourceId := "alicloud_vpc.default"
	ra := resourceAttrInit(resourceId, AlicloudVpcMap1)
	rc := resourceCheckInitWithDescribeMethod(resourceId, &v, func() interface{} {
		return &VpcService{testAccProvider.Meta().(*connectivity.AliyunClient)}
	}, "DescribeVpc")
	rac := resourceAttrCheckInit(rc, ra)
	testAccCheck := rac.resourceAttrMapUpdateSet()
	rand := acctest.RandIntRange(1000000, 9999999)
	name := fmt.Sprintf("tf-testAcc%sVpc%d", defaultRegionToTest, rand)
	testAccConfig := resourceTestAccConfigFunc(resourceId, name, AlicloudVpcBasicDependence1)
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckWithRegions(t, true, connectivity.VpcIpv6SupportRegions)
		},

		IDRefreshName: resourceId,
		Providers:     testAccProviders,
		CheckDestroy:  rac.checkResourceDestroy(),
		Steps: []resource.TestStep{
			{
				Config: testAccConfig(map[string]interface{}{
					"user_cidrs":  []string{"106.11.62.0/24"},
					"enable_ipv6": "true",
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"user_cidrs.#": "1",
						"enable_ipv6":  "true",
					}),
				),
			},
			{
				ResourceName:            resourceId,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"dry_run", "enable_ipv6"},
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"vpc_name": name,
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"vpc_name": name,
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"description": name,
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"description": name,
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"secondary_cidr_blocks": []string{"10.0.0.0/8"},
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"secondary_cidr_blocks.#": "1",
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"resource_group_id": "${data.alicloud_resource_manager_resource_groups.default.groups.0.id}",
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"resource_group_id": CHECKSET,
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"tags": map[string]string{
						"Created": "TF",
						"For":     "Test",
					},
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"tags.%":       "2",
						"tags.Created": "TF",
						"tags.For":     "Test",
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"tags": map[string]string{
						"Created": "TF-update",
						"For":     "Test-update",
					},
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"tags.%":       "2",
						"tags.Created": "TF-update",
						"tags.For":     "Test-update",
					}),
				),
			},
			{
				Config: testAccConfig(map[string]interface{}{
					"vpc_name":              name + "update",
					"description":           name + "update",
					"secondary_cidr_blocks": []string{"10.0.0.0/8"},
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"vpc_name":                name + "update",
						"description":             name + "update",
						"secondary_cidr_blocks.#": "1",
					}),
				),
			},
		},
	})
}

func TestAccAlicloudVpc_basic1(t *testing.T) {
	var v map[string]interface{}
	resourceId := "alicloud_vpc.default"
	ra := resourceAttrInit(resourceId, AlicloudVpcMap1)
	rc := resourceCheckInitWithDescribeMethod(resourceId, &v, func() interface{} {
		return &VpcService{testAccProvider.Meta().(*connectivity.AliyunClient)}
	}, "DescribeVpc")
	rac := resourceAttrCheckInit(rc, ra)
	testAccCheck := rac.resourceAttrMapUpdateSet()
	rand := acctest.RandIntRange(10000, 99999)
	name := fmt.Sprintf("tf-testAcc%sVpc%d", defaultRegionToTest, rand)
	testAccConfig := resourceTestAccConfigFunc(resourceId, name, AlicloudVpcBasicDependence1)
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckWithRegions(t, true, connectivity.VpcIpv6SupportRegions)
		},

		IDRefreshName: resourceId,
		Providers:     testAccProviders,
		CheckDestroy:  rac.checkResourceDestroy(),
		Steps: []resource.TestStep{
			{
				Config: testAccConfig(map[string]interface{}{
					"enable_ipv6":       "true",
					"vpc_name":          name,
					"description":       name,
					"resource_group_id": "${data.alicloud_resource_manager_resource_groups.default.groups.0.id}",
					"dry_run":           "false",
					"user_cidrs":        []string{"106.11.62.0/24"},
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"enable_ipv6":       "true",
						"vpc_name":          name,
						"description":       name,
						"resource_group_id": CHECKSET,
						"dry_run":           "false",
						"user_cidrs.#":      "1",
					}),
				),
			},
			{
				ResourceName:            resourceId,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"dry_run", "enable_ipv6"},
			},
		},
	})
}

func TestAccAlicloudVpc_basic2(t *testing.T) {
	var v map[string]interface{}
	resourceId := "alicloud_vpc.default"
	ra := resourceAttrInit(resourceId, AlicloudVpcMap1)
	rc := resourceCheckInitWithDescribeMethod(resourceId, &v, func() interface{} {
		return &VpcService{testAccProvider.Meta().(*connectivity.AliyunClient)}
	}, "DescribeVpc")
	rac := resourceAttrCheckInit(rc, ra)
	testAccCheck := rac.resourceAttrMapUpdateSet()
	rand := acctest.RandIntRange(10000, 99999)
	name := fmt.Sprintf("tf-testAcc%sVpc%d", defaultRegionToTest, rand)
	testAccConfig := resourceTestAccConfigFunc(resourceId, name, AlicloudVpcBasicDependence1)
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckWithRegions(t, true, connectivity.VpcIpv6SupportRegions)
		},

		IDRefreshName: resourceId,
		Providers:     testAccProviders,
		CheckDestroy:  rac.checkResourceDestroy(),
		Steps: []resource.TestStep{
			{
				Config: testAccConfig(map[string]interface{}{
					"enable_ipv6":       "true",
					"name":              name,
					"description":       name,
					"resource_group_id": "${data.alicloud_resource_manager_resource_groups.default.groups.0.id}",
					"dry_run":           "false",
					"user_cidrs":        []string{"106.11.62.0/24"},
				}),
				Check: resource.ComposeTestCheckFunc(
					testAccCheck(map[string]string{
						"enable_ipv6":       "true",
						"name":              name,
						"description":       name,
						"resource_group_id": CHECKSET,
						"dry_run":           "false",
						"user_cidrs.#":      "1",
					}),
				),
			},
			{
				ResourceName:            resourceId,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"dry_run", "enable_ipv6"},
			},
		},
	})
}

var AlicloudVpcMap1 = map[string]string{
	"status":          CHECKSET,
	"router_id":       CHECKSET,
	"router_table_id": CHECKSET,
	"route_table_id":  CHECKSET,
	"ipv6_cidr_block": CHECKSET,
}

func AlicloudVpcBasicDependence1(name string) string {
	return fmt.Sprintf(`

data "alicloud_resource_manager_resource_groups" "default" {
  name_regex = "default"
}
`)
}

func TestAccAlicloudVpc_unit(t *testing.T) {
	p := Provider().(*schema.Provider).ResourcesMap
	d, _ := schema.InternalMap(p["alicloud_vpc"].Schema).Data(nil, nil)
	dCreate, _ := schema.InternalMap(p["alicloud_vpc"].Schema).Data(nil, nil)
	dCreate.MarkNewResource()
	for key, value := range map[string]interface{}{
		"cidr_block":            "cidr_block",
		"classic_link_enabled":  false,
		"description":           "description",
		"dry_run":               false,
		"enable_ipv6":           false,
		"ip_version":            "ip_version",
		"ipv6_cidr_block":       "ipv6_cidr_block",
		"ipv6_isp":              "ipv6_isp",
		"resource_group_id":     "resource_group_id",
		"secondary_cidr_blocks": []interface{}{"secondary_cidr_blocks_1", "secondary_cidr_blocks_2"},
		"tags": []interface{}{
			map[string]interface{}{
				"tag_key":   "tag_key",
				"tag_value": "tag_value",
			},
		},
		"user_cidrs": []interface{}{"user_cidrs_1", "user_cidrs_2"},
		"vpc_name":   "vpc_name",
	} {
		err := dCreate.Set(key, value)
		assert.Nil(t, err)
		err = d.Set(key, value)
		assert.Nil(t, err)
	}
	region := os.Getenv("ALICLOUD_REGION")
	rawClient, err := sharedClientForRegion(region)
	if err != nil {
		t.Skipf("Skipping the test case with err: %s", err)
		t.Skipped()
	}
	rawClient = rawClient.(*connectivity.AliyunClient)
	ReadMockResponse := map[string]interface{}{
		// DescribeVpcAttribute
		"CidrBlock":          "cidr_block",
		"ClassicLinkEnabled": false,
		"CreationTime":       "create_time",
		"Description":        "description",
		"Ipv6CidrBlock":      "ipv6_cidr_block",
		"IsDefault":          false,
		"RegionId":           "region_id",
		"ResourceGroupId":    "resource_group_id",
		"VRouterId":          "router_id",
		"SecondaryCidrBlocks": map[string]interface{}{
			"SecondaryCidrBlock": []interface{}{
				"secondary_cidr_blocks_1",
				"secondary_cidr_blocks_2",
			},
		},
		"Status": "status",
		"UserCidrs": map[string]interface{}{
			"UserCidr": []interface{}{
				"user_cidrs_1",
				"user_cidrs_2",
			},
		},
		"VSwitchIds": map[string]interface{}{
			"VSwitchId": []interface{}{
				"vswitch_ids_1",
				"vswitch_ids_2",
			},
		},
		"VpcId":   "vpc_id",
		"VpcName": "vpc_name",
		// DescribeVpcs
		"Vpcs": map[string]interface{}{
			"Vpc": []interface{}{
				map[string]interface{}{
					"CidrBlock":       "cidr_block",
					"CreationTime":    "create_time",
					"Description":     "description",
					"Ipv6CidrBlock":   "ipv6_cidr_block",
					"IsDefault":       false,
					"RegionId":        "region_id",
					"ResourceGroupId": "resource_group_id",
					"VRouterId":       "router_id",
					"SecondaryCidrBlocks": map[string]interface{}{
						"SecondaryCidrBlock": []interface{}{
							"secondary_cidr_blocks_1",
							"secondary_cidr_blocks_2",
						},
					},
					"Status": "status",
					"Tags": map[string]interface{}{
						"Tag": []interface{}{
							map[string]interface{}{
								"Key":   "tag_key",
								"Value": "tag_value",
							},
						},
					},
					"UserCidrs": map[string]interface{}{
						"UserCidr": []interface{}{
							"user_cidrs_1",
							"user_cidrs_2",
						},
					},
					"VSwitchIds": map[string]interface{}{
						"VSwitchId": []interface{}{
							"vswitch_ids_1",
							"vswitch_ids_2",
						},
					},
					"VpcId":   "vpc_id",
					"VpcName": "vpc_name",
				},
			},
		},
	}
	CreateMockResponse := map[string]interface{}{
		// CreateVpc
		"ResourceGroupId": "resource_group_id",
		"VpcId":           "vpc_id",
	}
	responseMock := map[string]func(errorCode string) (map[string]interface{}, error){
		"RetryError": func(errorCode string) (map[string]interface{}, error) {
			return nil, &tea.SDKError{
				Code:    String(errorCode),
				Data:    String(errorCode),
				Message: String(errorCode),
			}
		},
		"NoRetryError": func(errorCode string) (map[string]interface{}, error) {
			return nil, &tea.SDKError{
				Code:    String(errorCode),
				Data:    String(errorCode),
				Message: String(errorCode),
			}
		},
		"CreateSuccess": func(errorCode string) (map[string]interface{}, error) {
			result := ReadMockResponse
			mapMerge(result, CreateMockResponse)
			return result, nil
		},
		"UpdateSuccess": func(errorCode string) (map[string]interface{}, error) {
			result := ReadMockResponse
			return result, nil
		},
		"DeleteSuccess": func(errorCode string) (map[string]interface{}, error) {
			result := ReadMockResponse
			return result, nil
		},
		"ReadSuccess": func(errorCode string) (map[string]interface{}, error) {
			result := ReadMockResponse
			return result, nil
		},
	}

	// Create
	t.Run("Create", func(t *testing.T) {
		patches := gomonkey.ApplyMethod(reflect.TypeOf(&connectivity.AliyunClient{}), "NewVpcClient", func(_ *connectivity.AliyunClient) (*client.Client, error) {
			return nil, &tea.SDKError{
				Code:    String("loadEndpoint error"),
				Data:    String("loadEndpoint error"),
				Message: String("loadEndpoint error"),
			}
		})
		err := resourceAlicloudVpcCreate(d, rawClient)
		patches.Reset()
		assert.NotNil(t, err)
		for _, errorCode := range []string{"TaskConflict", "Throttling", "UnknownError", "NonRetryableError", "nil"} {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(&client.Client{}), "DoRequest", func(_ *client.Client, _ *string, _ *string, _ *string, _ *string, _ *string, _ map[string]interface{}, _ map[string]interface{}, _ *util.RuntimeOptions) (map[string]interface{}, error) {
				switch errorCode {
				case "nil":
					return responseMock["CreateSuccess"]("")
				case "NonRetryableError":
					return responseMock["NoRetryError"](errorCode)
				default:
					return responseMock["RetryError"](errorCode)
				}
			})
			err := resourceAlicloudVpcCreate(dCreate, rawClient)
			patches.Reset()
			if errorCode != "nil" {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				for key, _ := range attributes {
					assert.False(t, dCreate.HasChange(key))
				}
			}
		}
	})

	// Update
	t.Run("Update", func(t *testing.T) {
		patches := gomonkey.ApplyMethod(reflect.TypeOf(&connectivity.AliyunClient{}), "NewVpcClient", func(_ *connectivity.AliyunClient) (*client.Client, error) {
			return nil, &tea.SDKError{
				Code:    String("loadEndpoint error"),
				Data:    String("loadEndpoint error"),
				Message: String("loadEndpoint error"),
			}
		})
		err := resourceAlicloudVpcUpdate(d, rawClient)
		patches.Reset()
		assert.NotNil(t, err)
		// ModifyVpcAttribute
		diff := terraform.NewInstanceDiff()
		diff.SetAttribute("cidr_block", &terraform.ResourceAttrDiff{Old: d.Get("cidr_block").(string), New: d.Get("cidr_block").(string) + "_update"})
		diff.SetAttribute("description", &terraform.ResourceAttrDiff{Old: d.Get("description").(string), New: d.Get("description").(string) + "_update"})
		diff.SetAttribute("enable_ipv6", &terraform.ResourceAttrDiff{Old: fmt.Sprint(d.Get("enable_ipv6")), New: fmt.Sprint(!d.Get("enable_ipv6").(bool))})
		diff.SetAttribute("ipv6_cidr_block", &terraform.ResourceAttrDiff{Old: d.Get("ipv6_cidr_block").(string), New: d.Get("ipv6_cidr_block").(string) + "_update"})
		diff.SetAttribute("ipv6_isp", &terraform.ResourceAttrDiff{Old: d.Get("ipv6_isp").(string), New: d.Get("ipv6_isp").(string) + "_update"})
		diff.SetAttribute("vpc_name", &terraform.ResourceAttrDiff{Old: d.Get("vpc_name").(string), New: d.Get("vpc_name").(string) + "_update"})
		dUpdate, _ := schema.InternalMap(p["alicloud_vpc"].Schema).Data(nil, diff)
		for _, errorCode := range []string{"Throttling", "NonRetryableError", "nil"} {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(&client.Client{}), "DoRequest", func(_ *client.Client, _ *string, _ *string, _ *string, _ *string, _ *string, _ map[string]interface{}, _ map[string]interface{}, _ *util.RuntimeOptions) (map[string]interface{}, error) {
				switch errorCode {
				case "nil":
					return responseMock["UpdateSuccess"]("")
				case "NonRetryableError":
					return responseMock["NoRetryError"](errorCode)
				default:
					return responseMock["RetryError"](errorCode)
				}
			})
			err := resourceAlicloudVpcUpdate(dUpdate, rawClient)
			patches.Reset()
			if errorCode != "nil" {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				for key, _ := range attributes {
					assert.False(t, dUpdate.HasChange(key))
				}
			}
		}
		// MoveResourceGroup
		diff = terraform.NewInstanceDiff()
		diff.SetAttribute("resource_group_id", &terraform.ResourceAttrDiff{Old: d.Get("resource_group_id").(string), New: d.Get("resource_group_id").(string) + "_update"})
		dUpdate, _ = schema.InternalMap(p["alicloud_vpc"].Schema).Data(nil, diff)
		for _, errorCode := range []string{"Throttling", "NonRetryableError", "nil"} {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(&client.Client{}), "DoRequest", func(_ *client.Client, _ *string, _ *string, _ *string, _ *string, _ *string, _ map[string]interface{}, _ map[string]interface{}, _ *util.RuntimeOptions) (map[string]interface{}, error) {
				switch errorCode {
				case "nil":
					return responseMock["UpdateSuccess"]("")
				case "NonRetryableError":
					return responseMock["NoRetryError"](errorCode)
				default:
					return responseMock["RetryError"](errorCode)
				}
			})
			err := resourceAlicloudVpcUpdate(dUpdate, rawClient)
			patches.Reset()
			if errorCode != "nil" {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				for key, _ := range attributes {
					assert.False(t, dUpdate.HasChange(key))
				}
			}
		}
		// DisableVpcClassicLink
		diff = terraform.NewInstanceDiff()
		diff.SetAttribute("classic_link_enabled", &terraform.ResourceAttrDiff{Old: d.Get("classic_link_enabled").(string), New: "false"})
		dUpdate, _ = schema.InternalMap(p["alicloud_vpc"].Schema).Data(nil, diff)
		for _, errorCode := range []string{"IncorrectVpcStatus", "InternalError", "NonRetryableError", "nil"} {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(&client.Client{}), "DoRequest", func(_ *client.Client, _ *string, _ *string, _ *string, _ *string, _ *string, _ map[string]interface{}, _ map[string]interface{}, _ *util.RuntimeOptions) (map[string]interface{}, error) {
				switch errorCode {
				case "nil":
					return responseMock["UpdateSuccess"]("")
				case "NonRetryableError":
					return responseMock["NoRetryError"](errorCode)
				default:
					return responseMock["RetryError"](errorCode)
				}
			})
			err := resourceAlicloudVpcUpdate(dUpdate, rawClient)
			patches.Reset()
			if errorCode != "nil" {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				for key, _ := range attributes {
					assert.False(t, dUpdate.HasChange(key))
				}
			}
		}
		// EnableVpcClassicLink
		diff = terraform.NewInstanceDiff()
		diff.SetAttribute("classic_link_enabled", &terraform.ResourceAttrDiff{Old: d.Get("classic_link_enabled").(string), New: "true"})
		dUpdate, _ = schema.InternalMap(p["alicloud_vpc"].Schema).Data(nil, diff)
		for _, errorCode := range []string{"IncorrectVpcStatus", "NonRetryableError", "nil"} {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(&client.Client{}), "DoRequest", func(_ *client.Client, _ *string, _ *string, _ *string, _ *string, _ *string, _ map[string]interface{}, _ map[string]interface{}, _ *util.RuntimeOptions) (map[string]interface{}, error) {
				switch errorCode {
				case "nil":
					return responseMock["UpdateSuccess"]("")
				case "NonRetryableError":
					return responseMock["NoRetryError"](errorCode)
				default:
					return responseMock["RetryError"](errorCode)
				}
			})
			err := resourceAlicloudVpcUpdate(dUpdate, rawClient)
			patches.Reset()
			if errorCode != "nil" {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				for key, _ := range attributes {
					assert.False(t, dUpdate.HasChange(key))
				}
			}
		}
		// UnassociateVpcCidrBlock
		diff = terraform.NewInstanceDiff()
		diff.SetAttribute("secondary_cidr_blocks.0", &terraform.ResourceAttrDiff{Old: d.Get("secondary_cidr_blocks").([]interface{})[0].(string), New: d.Get("secondary_cidr_blocks").([]interface{})[0].(string) + "_update"})
		diff.SetAttribute("secondary_cidr_blocks.1", &terraform.ResourceAttrDiff{Old: d.Get("secondary_cidr_blocks").([]interface{})[1].(string), New: d.Get("secondary_cidr_blocks").([]interface{})[1].(string) + "_update"})
		dUpdate, _ = schema.InternalMap(p["alicloud_vpc"].Schema).Data(nil, diff)
		for _, errorCode := range []string{"Throttling", "NonRetryableError", "nil"} {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(&client.Client{}), "DoRequest", func(_ *client.Client, _ *string, _ *string, _ *string, _ *string, _ *string, _ map[string]interface{}, _ map[string]interface{}, _ *util.RuntimeOptions) (map[string]interface{}, error) {
				switch errorCode {
				case "nil":
					return responseMock["UpdateSuccess"]("")
				case "NonRetryableError":
					return responseMock["NoRetryError"](errorCode)
				default:
					return responseMock["RetryError"](errorCode)
				}
			})
			err := resourceAlicloudVpcUpdate(dUpdate, rawClient)
			patches.Reset()
			if errorCode != "nil" {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				for key, _ := range attributes {
					assert.False(t, dUpdate.HasChange(key))
				}
			}
		}
		// AssociateVpcCidrBlock
		diff = terraform.NewInstanceDiff()
		diff.SetAttribute("ip_version", &terraform.ResourceAttrDiff{Old: d.Get("ip_version").(string), New: d.Get("ip_version").(string) + "_update"})
		diff.SetAttribute("ipv6_isp", &terraform.ResourceAttrDiff{Old: d.Get("ipv6_isp").(string), New: d.Get("ipv6_isp").(string) + "_update"})
		diff.SetAttribute("secondary_cidr_blocks.0", &terraform.ResourceAttrDiff{Old: d.Get("secondary_cidr_blocks").([]interface{})[0].(string), New: d.Get("secondary_cidr_blocks").([]interface{})[0].(string) + "_update"})
		diff.SetAttribute("secondary_cidr_blocks.1", &terraform.ResourceAttrDiff{Old: d.Get("secondary_cidr_blocks").([]interface{})[1].(string), New: d.Get("secondary_cidr_blocks").([]interface{})[1].(string) + "_update"})
		dUpdate, _ = schema.InternalMap(p["alicloud_vpc"].Schema).Data(nil, diff)
		for _, errorCode := range []string{"Throttling", "NonRetryableError", "nil"} {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(&client.Client{}), "DoRequest", func(_ *client.Client, _ *string, _ *string, _ *string, _ *string, _ *string, _ map[string]interface{}, _ map[string]interface{}, _ *util.RuntimeOptions) (map[string]interface{}, error) {
				switch errorCode {
				case "nil":
					return responseMock["UpdateSuccess"]("")
				case "NonRetryableError":
					return responseMock["NoRetryError"](errorCode)
				default:
					return responseMock["RetryError"](errorCode)
				}
			})
			err := resourceAlicloudVpcUpdate(dUpdate, rawClient)
			patches.Reset()
			if errorCode != "nil" {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				for key, _ := range attributes {
					assert.False(t, dUpdate.HasChange(key))
				}
			}
		}
		// UnTagResources
		diff = terraform.NewInstanceDiff()
		diff.SetAttribute("tags.%", &terraform.ResourceAttrDiff{Old: "0", New: "2"})
		diff.SetAttribute("tags.For", &terraform.ResourceAttrDiff{Old: "", New: "Test"})
		diff.SetAttribute("tags.Created", &terraform.ResourceAttrDiff{Old: "", New: "TF"})
		dUpdate, _ = schema.InternalMap(p["alicloud_vpc"].Schema).Data(nil, diff)
		for _, errorCode := range []string{"Throttling", "NonRetryableError", "nil"} {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(&client.Client{}), "DoRequest", func(_ *client.Client, _ *string, _ *string, _ *string, _ *string, _ *string, _ map[string]interface{}, _ map[string]interface{}, _ *util.RuntimeOptions) (map[string]interface{}, error) {
				switch errorCode {
				case "nil":
					return responseMock["UpdateSuccess"]("")
				case "NonRetryableError":
					return responseMock["NoRetryError"](errorCode)
				default:
					return responseMock["RetryError"](errorCode)
				}
			})
			err := resourceAlicloudVpcUpdate(dUpdate, rawClient)
			patches.Reset()
			if errorCode != "nil" {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				for key, _ := range attributes {
					assert.False(t, dUpdate.HasChange(key))
				}
			}
		}
		// TagResources
		diff = terraform.NewInstanceDiff()
		diff.SetAttribute("tags.%", &terraform.ResourceAttrDiff{Old: "0", New: "2"})
		diff.SetAttribute("tags.For", &terraform.ResourceAttrDiff{Old: "", New: "Test"})
		diff.SetAttribute("tags.Created", &terraform.ResourceAttrDiff{Old: "", New: "TF"})
		dUpdate, _ = schema.InternalMap(p["alicloud_vpc"].Schema).Data(nil, diff)
		for _, errorCode := range []string{"Throttling", "NonRetryableError", "nil"} {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(&client.Client{}), "DoRequest", func(_ *client.Client, _ *string, _ *string, _ *string, _ *string, _ *string, _ map[string]interface{}, _ map[string]interface{}, _ *util.RuntimeOptions) (map[string]interface{}, error) {
				switch errorCode {
				case "nil":
					return responseMock["UpdateSuccess"]("")
				case "NonRetryableError":
					return responseMock["NoRetryError"](errorCode)
				default:
					return responseMock["RetryError"](errorCode)
				}
			})
			err := resourceAlicloudVpcUpdate(dUpdate, rawClient)
			patches.Reset()
			if errorCode != "nil" {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				for key, _ := range attributes {
					assert.False(t, dUpdate.HasChange(key))
				}
			}
		}
	})

	// Delete
	t.Run("Delete", func(t *testing.T) {
		patches := gomonkey.ApplyMethod(reflect.TypeOf(&connectivity.AliyunClient{}), "NewVpcClient", func(_ *connectivity.AliyunClient) (*client.Client, error) {
			return nil, &tea.SDKError{
				Code:    String("loadEndpoint error"),
				Data:    String("loadEndpoint error"),
				Message: String("loadEndpoint error"),
			}
		})
		err := resourceAlicloudVpcDelete(d, rawClient)
		patches.Reset()
		assert.NotNil(t, err)

		for _, errorCode := range []string{"Throttling", "Forbidden.VpcNotFound", "InvalidVpcID.NotFound", "NonRetryableError", "nil"} {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(&client.Client{}), "DoRequest", func(_ *client.Client, _ *string, _ *string, _ *string, _ *string, _ *string, _ map[string]interface{}, _ map[string]interface{}, _ *util.RuntimeOptions) (map[string]interface{}, error) {
				switch errorCode {
				case "nil":
					return responseMock["DeleteSuccess"]("")
				case "NonRetryableError", "Forbidden.VpcNotFound", "InvalidVpcID.NotFound":
					return responseMock["NoRetryError"](errorCode)
				default:
					return responseMock["RetryError"](errorCode)
				}
			})
			err := resourceAlicloudVpcDelete(d, rawClient)
			patches.Reset()
			if errorCode != "nil" {
				isNotFoundError := false
				for _, notFoundErrorCode := range []string{"Forbidden.VpcNotFound", "InvalidVpcID.NotFound"} {
					if errorCode == notFoundErrorCode {
						isNotFoundError = true
						break
					}
				}
				if isNotFoundError {
					assert.Nil(t, err)
				} else {
					assert.NotNil(t, err)
				}
			} else {
				assert.Nil(t, err)
			}
		}
	})
}
