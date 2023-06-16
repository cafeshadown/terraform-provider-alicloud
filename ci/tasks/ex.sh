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
myDir="$(cd $(dirname $0) && pwd)"
#/tmp/build/1ab8e1b6/terraform-provider-alicloud/ci/tasks
releaseDir="$(cd ${myDir} && cd ../.. && pwd)"

#/terraform-provider-alicloud/website/docs/r/oss_bucket.html.markdown
# shellcheck disable=SC1007

cd $releaseDir
changeFiles=$(git diff --name-only HEAD^ HEAD | grep "^alicloud/" | grep ".go$" | grep -v ".markdown$")
docsFiles=$(git diff --name-only HEAD^ HEAD | grep ".markdown$")
diffFiles=(${docsFiles[@]} ${changeFiles[@]})
noNeedRun=true
exampleCount=0
if [[ ${#diffFiles[@]} -eq 0 ]]; then
  echo -e "\033[33m[WARNING]\033[0m the pr ${prNum} does not change provider code and there is no need to check."
  exit 0
fi
for fileName in ${diffFiles[@]}; do
  if [[ ${fileName} == "alicloud/resource_alicloud"* || ${fileName} == "alicloud/data_source_alicloud"* || ${fileName} == "website/docs/r/"* ]]; then
    echo ${fileName}
    if [[ ${fileName} == *".go" ]]; then
      fileName=(${fileName/.go/.html.markdown})
      fileName=(${fileName#*resource_alicloud_})
    fi
    if [[ ${fileName} == *?.html.markdown ]]; then
      fileName=(${fileName#*r/})
    fi
    resourceName=${fileName%%.html.markdown}
    noNeedRun=false
    if [[ $(grep -c '```terraform' "website/docs/r/${resourceName}.html.markdown") -lt 1 ]]; then
      echo -e "\033[33m[WARNING]\033[0m missing the acceptance examples in the $resourceName, continue..."
      continue
    fi
    diffExampleCount=$(grep -c '```terraform' "website/docs/r/${resourceName}.html.markdown")
    echo -e "found the example count:\n${diffExampleCount}"
    exampleCount=$(($exampleCount + $diffExampleCount))
  fi
done

if [[ "${noNeedRun}" = "false" && ${exampleCount} == "0" ]]; then
  echo -e "\033[31mthe pr ${prNum} missing docs example, please adding them. \033[0m"
  exit 1
fi
if [[ "${noNeedRun}" = "true" ]]; then
  echo -e "\n\033[33m[WARNING]\033[0m the pr is no need to run example.\033[0m"
  exit 0
fi

# init example tf
# shellcheck disable=SC2034

#make dev
#tar -xvf bin/terraform-provider-alicloud_darwin-amd64.tgz
#export TFNV=1.200.0
#rm -rf ~/.terraform.d/plugin-cache/registry.terraform.io/hashicorp/alicloud/${TFNV}/darwin_amd64/
#mkdir -p ~/.terraform.d/plugin-cache/registry.terraform.io/hashicorp/alicloud/${TFNV}/darwin_amd64/
#mv bin/terraform-provider-alicloud ~/.terraform.d/plugin-cache/registry.terraform.io/hashicorp/alicloud/${TFNV}/darwin_amd64/terraform-provider-alicloud_v${TFNV}

exampleTerraformErrorLog=terraform-example.run.error.log
exampleTerraformErrorTmpLog=terraform-example.error.temp.log
exampleTerraformLog=terraform-example.run.log
exampleRunLog=terraform-example.run.result.log

for fileName in ${diffFiles[@]}; do
  if [[ ${fileName} == "alicloud/resource_alicloud"* || ${fileName} == "alicloud/data_source_alicloud"* || ${fileName} == "website/docs/r/"* ]]; then
    echo ${fileName}
    if [[ ${fileName} == *".go" ]]; then
      fileName=(${fileName/.go/.html.markdown})
      fileName=(${fileName#*resource_alicloud_})
    fi
    if [[ ${fileName} == *?.html.markdown ]]; then
      fileName=(${fileName#*r/})
    fi
    resourceName=${fileName%%.html.markdown}
    #run example
    begin=false
    count=0
    docsDir="website/docs/r/${resourceName}.html.markdown"
    cat ${docsDir} | while read line; do
      exampleFileName="${resourceName}_example_${count}"
      exampleTerraformContent=${exampleFileName}/main.tf

      if [[ $line == '```terraform' ]]; then
        begin=true
        #    create file
        if [ ! -d $exampleFileName ]; then
          mkdir $exampleFileName
          cp -rf ci/tasks/docs-example.tf $exampleFileName/terraform.tf
        fi
        #clear file
        if [ ! -d $exampleTerraformContent ]; then
          echo "" >${exampleTerraformContent}
        fi
        continue
      fi
      #  end
      if [[ $line == '```' && "${begin}" = "true" ]]; then
        begin=false
        echo "=== RUN   ${exampleFileName} APPLY" | tee -a ${exampleTerraformLog} ${exampleRunLog}
        #    terraform apply
        { terraform -chdir=${exampleFileName} init && terraform -chdir=${exampleFileName} plan && terraform -chdir=${exampleFileName} apply -auto-approve; } 2>${exampleTerraformErrorTmpLog} >>${exampleTerraformLog}

        if [ $? -ne 0 ]; then
          cat ${exampleTerraformErrorTmpLog} >>${exampleTerraformErrorLog}
          echo "--- FAIL: ${exampleFileName}" >>${exampleRunLog}
        else
          echo "--- PASS: ${exampleFileName}" >>${exampleRunLog}
        fi
        echo "=== RUN   ${exampleFileName} DESTROY" | tee -a ${exampleTerraformLog} ${exampleRunLog}
        #   terraform destroy
        # shellcheck disable=SC1083
        { terraform -chdir=${exampleFileName} plan -destroy && terraform -chdir=${exampleFileName} apply -destroy -auto-approve; } 2>${exampleTerraformErrorTmpLog} >>${exampleTerraformLog}

        if [ $? -ne 0 ]; then
          cat ${exampleTerraformErrorTmpLog} >>${exampleTerraformErrorLog}
          echo "--- FAIL: ${exampleFileName}" >>${exampleRunLog}
        else
          echo "--- PASS: ${exampleFileName}" >>${exampleRunLog}
        fi
        let "count=count+1"
        continue
      fi
      if [[ "${begin}" = "true" ]]; then
        echo -e "${line}" >>${exampleTerraformContent}
      fi
    done
  fi
done

aliyun oss cp ${exampleRunLog} oss://${OSS_BUCKET_NAME}/${ossObjectPath}/${exampleRunLog} -f --access-key-id ${ALICLOUD_ACCESS_KEY} --access-key-secret ${ALICLOUD_SECRET_KEY} --region ${OSS_BUCKET_REGION}
if [[ "$?" != "0" ]]; then
  echo -e "\033[31m uploading the pr ${prNum} example run log  to oss failed, please checking it.\033[0m"
  exit 1
fi
aliyun oss cp ${exampleTerraformLog} oss://${OSS_BUCKET_NAME}/${ossObjectPath}/${exampleTerraformLog} -f --access-key-id ${ALICLOUD_ACCESS_KEY} --access-key-secret ${ALICLOUD_SECRET_KEY} --region ${OSS_BUCKET_REGION}
if [[ "$?" != "0" ]]; then
  echo -e "\033[31m uploading the pr ${prNum} example run log  to oss failed, please checking it.\033[0m"
  exit 1
fi
aliyun oss cp ${exampleTerraformErrorLog} oss://${OSS_BUCKET_NAME}/${ossObjectPath}/${exampleTerraformErrorLog} -f --access-key-id ${ALICLOUD_ACCESS_KEY} --access-key-secret ${ALICLOUD_SECRET_KEY} --region ${OSS_BUCKET_REGION}
if [[ "$?" != "0" ]]; then
  echo -e "\033[31m uploading the pr ${prNum} example run log  to oss failed, please checking it.\033[0m"
  exit 1
fi
