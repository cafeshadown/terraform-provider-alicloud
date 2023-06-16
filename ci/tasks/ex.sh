#!/usr/bin/env bash

: ${ALICLOUD_ACCESS_KEY:?}
: ${ALICLOUD_SECRET_KEY:?}
: ${ALICLOUD_ACCOUNT_ID:1511928242963727}
: ${DING_TALK_TOKEN:=""}
: ${OSS_BUCKET_NAME:=?}
: ${OSS_BUCKET_REGION:=?}
#: ${GITHUB_TOKEN:?}

repo=terraform-provider-alicloud
export GITHUB_TOKEN=${GITHUB_TOKEN}
export GH_REPO=aliyun/${repo}

#/tmp/build/1ab8e1b6/terraform-provider-alicloud
my_dir="$(cd $(dirname $0) && pwd)"
#/tmp/build/1ab8e1b6/terraform-provider-alicloud/ci/tasks
release_dir="$(cd ${my_dir} && cd ../.. && pwd)"

docs_dir=${release_dir}"/website/docs/r/oss_bucket.html.markdown"
#/terraform-provider-alicloud/website/docs/r/oss_bucket.html.markdown
# shellcheck disable=SC1007

resource_name="oss_bucket"
# init example tf
begin=false
count=0
# shellcheck disable=SC2089
terraform_provider_version_config="terraform {
                                         required_providers {
                                           alicloud = {
                                             source  = \"hashicorp/alicloud\"
                                             version = \"1.207.2\"
                                           }
                                         }
                                       }"

cat ${docs_dir} | while read line; do
  example_file_name="${resource_name}_example_${count}"
  example_terraform_content=${example_file_name}/main.tf
  example_terraform_log=${example_file_name}/terraform.run.raw.log
  example_terraform_run_log=${example_file_name}/terraform.run.log
  if [[ $line == '```terraform' ]]; then

    begin=true
    #    create file
    if [ ! -d $example_file_name ]; then
      mkdir $example_file_name
    fi
    #    clear file
    if [ ! -d $example_terraform_content ]; then
      echo "" >${example_terraform_content}
    fi
    if [ ! -d ${example_terraform_log} ]; then
      echo "" >${example_terraform_log}
    fi
      if [ ! -d ${example_terraform_log} ]; then
          echo "" >${example_terraform_log}
        fi
    echo -e "${example_terraform_run_log}" >>${example_terraform_content}
    continue
  fi
  #  end
  if [[ $line == '```' && "${begin}" = "true" ]]; then
    begin=false
    echo "=== RUN   ${example_file_name}" >>${example_terraform_run_log}

    #    terraform init
    terraform -chdir=${example_file_name} plan >${example_terraform_log} 2>&1

    if [ $? -ne 0 ]; then
      echo "--- FAIL: ${example_file_name}">>${example_terraform_run_log}
    else
      echo "--- PASS: ${example_file_name}">>${example_terraform_run_log}
    fi
    let "count=count+1"
    continue
  fi
  if [[ "${begin}" = "true" ]]; then

    echo -e "${line}" >>${example_terraform_content}
  fi
done
