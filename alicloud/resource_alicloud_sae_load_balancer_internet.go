package alicloud

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"strconv"
	"time"

	util "github.com/alibabacloud-go/tea-utils/service"
	"github.com/aliyun/terraform-provider-alicloud/alicloud/connectivity"
	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func resourceAlicloudSaeLoadBalancerInternet() *schema.Resource {
	return &schema.Resource{
		Create: resourceAlicloudSaeSaeLoadBalancerInternetCreate,
		Read:   resourceAlicloudSaeSaeLoadBalancerInternetRead,
		Update: resourceAlicloudSaeSaeLoadBalancerInternetUpdate,
		Delete: resourceAlicloudSaeSaeLoadBalancerInternetDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: map[string]*schema.Schema{
			"app_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"internet_slb_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"internet": {
				Type:     schema.TypeSet,
				Required: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"https_cert_id": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"protocol": {
							Type:         schema.TypeString,
							ValidateFunc: validation.StringInSlice([]string{"TCP", "HTTP", "HTTPS"}, false),
							Optional:     true,
						},
						"target_port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
						"port": {
							Type:     schema.TypeInt,
							Optional: true,
						},
					},
				},
			},
			"internet_ip": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceAlicloudSaeSaeLoadBalancerInternetCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*connectivity.AliyunClient)
	saeService := SaeService{client}
	var response map[string]interface{}
	action := "/pop/v1/sam/app/slb"
	request := make(map[string]*string)
	conn, err := client.NewServerlessClient()
	if err != nil {
		return WrapError(err)
	}
	request["AppId"] = StringPointer(d.Get("app_id").(string))
	request["InternetSlbId"] = StringPointer(d.Get("internet_slb_id").(string))
	for _, internet := range d.Get("internet").(*schema.Set).List() {
		internetMap := internet.(map[string]interface{})
		internetReq := []interface{}{
			map[string]interface{}{
				"httpsCertId": internetMap["https_cert_id"],
				"protocol":    internetMap["protocol"],
				"targetPort":  internetMap["target_port"],
				"port":        internetMap["port"],
			},
		}
		obj, err := json.Marshal(internetReq)
		if err != nil {
			return WrapError(err)
		}
		request["Internet"] = StringPointer(string(obj))
	}

	wait := incrementalWait(3*time.Second, 3*time.Second)
	err = resource.Retry(d.Timeout(schema.TimeoutUpdate), func() *resource.RetryError {
		response, err = conn.DoRequest(StringPointer("2019-05-06"), nil, StringPointer("POST"), StringPointer("AK"), StringPointer(action), request, nil, nil, &util.RuntimeOptions{})
		if err != nil {
			if IsExpectedErrors(err, []string{"Application.InvalidStatus", "Application.ChangerOrderRunning"}) || NeedRetry(err) {
				wait()
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})
	addDebug(action, response, request)
	if err != nil {
		return WrapErrorf(err, DefaultErrorMsg, d.Id(), "POST "+action, AlibabaCloudSdkGoERROR)
	}
	if respBody, isExist := response["body"]; isExist {
		response = respBody.(map[string]interface{})
	} else {
		return WrapError(fmt.Errorf("%s failed, response: %v", "POST "+action, response))
	}
	if fmt.Sprint(response["Success"]) == "false" {
		return WrapError(fmt.Errorf("%s failed, response: %v", "POST "+action, response))
	}
	d.SetId(fmt.Sprint(d.Get("app_id"), ":", d.Get("internet_slb_id")))

	stateConf := BuildStateConf([]string{}, []string{"SUCCESS"}, d.Timeout(schema.TimeoutUpdate), 5*time.Second, saeService.SaeApplicationStateRefreshFunc(d.Get("app_id").(string), []string{"FAIL", "AUTO_BATCH_WAIT", "APPROVED", "WAIT_APPROVAL", "WAIT_BATCH_CONFIRM", "ABORT", "SYSTEM_FAIL"}))
	if _, err := stateConf.WaitForState(); err != nil {
		return WrapErrorf(err, IdMsg, d.Id())
	}
	return resourceAlicloudSaeSaeLoadBalancerInternetRead(d, meta)
}
func resourceAlicloudSaeSaeLoadBalancerInternetRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*connectivity.AliyunClient)
	saeService := SaeService{client}
	parts, err := ParseResourceId(d.Id(), 2)
	if err != nil {
		return WrapError(err)
	}

	describeApplicationSlbObject, err := saeService.DescribeApplicationSlb(d.Id())
	if err != nil {
		return WrapError(err)
	}

	d.Set("internet_ip", describeApplicationSlbObject["InternetIp"])
	d.Set("internet_slb_id", describeApplicationSlbObject["InternetSlbId"])
	d.Set("app_id", parts[0])
	internetArray := make([]interface{}, 0)
	if v, ok := describeApplicationSlbObject["Internet"]; ok {
		for _, internet := range v.([]interface{}) {
			internetObject := internet.(map[string]interface{})
			internetObj := map[string]interface{}{
				"https_cert_id": internetObject["HttpsCertId"],
				"protocol":      internetObject["Protocol"],
				"target_port":   internetObject["TargetPort"],
				"port":          internetObject["Port"],
			}
			internetArray = append(internetArray, internetObj)
		}
	}
	d.Set("internet", internetArray)
	return nil
}
func resourceAlicloudSaeSaeLoadBalancerInternetUpdate(d *schema.ResourceData, meta interface{}) error {
	parts, err := ParseResourceId(d.Id(), 2)
	if err != nil {
		return WrapError(err)
	}
	client := meta.(*connectivity.AliyunClient)
	saeService := SaeService{client}
	conn, err := client.NewServerlessClient()
	if err != nil {
		return WrapError(err)
	}
	var response map[string]interface{}
	update := false
	request := map[string]*string{
		"AppId": StringPointer(parts[0]),
	}
	if d.HasChange("internet_slb_id") {
		update = true
	}
	request["InternetSlbId"] = StringPointer(d.Get("internet_slb_id").(string))

	if d.HasChange("internet") {
		update = true
	}
	for _, internet := range d.Get("internet").(*schema.Set).List() {
		internetMap := internet.(map[string]interface{})
		internetReq := []interface{}{
			map[string]interface{}{
				"httpsCertId": internetMap["https_cert_id"],
				"protocol":    internetMap["protocol"],
				"targetPort":  internetMap["target_port"],
				"port":        internetMap["port"],
			},
		}
		obj, err := json.Marshal(internetReq)
		if err != nil {
			return WrapError(err)
		}
		request["Internet"] = StringPointer(string(obj))
	}

	if update {
		action := "/pop/v1/sam/app/slb"
		wait := incrementalWait(3*time.Second, 3*time.Second)
		err = resource.Retry(d.Timeout(schema.TimeoutUpdate), func() *resource.RetryError {
			response, err = conn.DoRequest(StringPointer("2019-05-06"), nil, StringPointer("POST"), StringPointer("AK"), StringPointer(action), request, nil, nil, &util.RuntimeOptions{})
			if err != nil {
				if IsExpectedErrors(err, []string{"Application.InvalidStatus", "Application.ChangerOrderRunning"}) || NeedRetry(err) {
					wait()
					return resource.RetryableError(err)
				}
				return resource.NonRetryableError(err)
			}
			return nil
		})
		if err != nil {
			return WrapErrorf(err, DefaultErrorMsg, d.Id(), "POST "+action, AlibabaCloudSdkGoERROR)
		}
		if respBody, isExist := response["body"]; isExist {
			response = respBody.(map[string]interface{})
		} else {
			return WrapError(fmt.Errorf("%s failed, response: %v", "POST "+action, response))
		}
		addDebug(action, response, request)

		if fmt.Sprint(response["Success"]) == "false" {
			return WrapError(fmt.Errorf("%s failed, response: %v", action, response))
		}
	}
	stateConf := BuildStateConf([]string{}, []string{"SUCCESS"}, d.Timeout(schema.TimeoutUpdate), 5*time.Second, saeService.SaeApplicationStateRefreshFunc(d.Get("app_id").(string), []string{"FAIL", "AUTO_BATCH_WAIT", "APPROVED", "WAIT_APPROVAL", "WAIT_BATCH_CONFIRM", "ABORT", "SYSTEM_FAIL"}))
	if _, err := stateConf.WaitForState(); err != nil {
		return WrapErrorf(err, IdMsg, d.Id())
	}
	return resourceAlicloudSaeSaeLoadBalancerInternetRead(d, meta)
}
func resourceAlicloudSaeSaeLoadBalancerInternetDelete(d *schema.ResourceData, meta interface{}) error {
	parts, err := ParseResourceId(d.Id(), 2)
	if err != nil {
		return WrapError(err)
	}
	request := map[string]*string{
		"AppId":    StringPointer(parts[0]),
		"Internet": StringPointer(strconv.FormatBool(true)),
	}
	client := meta.(*connectivity.AliyunClient)
	conn, err := client.NewServerlessClient()
	if err != nil {
		return WrapError(err)
	}

	action := "/pop/v1/sam/app/slb"
	wait := incrementalWait(3*time.Second, 3*time.Second)
	var response map[string]interface{}
	err = resource.Retry(d.Timeout(schema.TimeoutUpdate), func() *resource.RetryError {
		response, err = conn.DoRequest(StringPointer("2019-05-06"), nil, StringPointer("DELETE"), StringPointer("AK"), StringPointer(action), request, nil, nil, &util.RuntimeOptions{})
		if err != nil {
			if IsExpectedErrors(err, []string{"Application.InvalidStatus", "Application.ChangerOrderRunning"}) || NeedRetry(err) {
				wait()
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return nil
	})
	addDebug(action, response, request)
	if err != nil {
		return WrapErrorf(err, DefaultErrorMsg, d.Id(), "POST "+action, AlibabaCloudSdkGoERROR)
	}
	if respBody, isExist := response["body"]; isExist {
		response = respBody.(map[string]interface{})
	} else {
		return WrapError(fmt.Errorf("%s failed, response: %v", "DELETE "+action, response))
	}
	if fmt.Sprint(response["Success"]) == "false" {
		return WrapError(fmt.Errorf("%s failed, response: %v", "DELETE "+action, response))
	}
	return nil
}
