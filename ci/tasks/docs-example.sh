#!/usr/bin/env bash

: ${ALICLOUD_ACCESS_KEY:?}
: ${ALICLOUD_SECRET_KEY:?}
: ${ALICLOUD_ACCOUNT_ID:?}
: ${DING_TALK_TOKEN:=""}
: ${OSS_BUCKET_NAME:=?}
: ${OSS_BUCKET_REGION:=?}
: ${GITHUB_TOKEN:?}

repo=terraform-provider-alicloud
export GITHUB_TOKEN=${GITHUB_TOKEN}
export GH_REPO=aliyun/${repo}

my_dir="$(cd $(dirname $0) && pwd)"
release_dir="$(cd ${my_dir} && cd ../.. && pwd)"

source ${release_dir}/ci/tasks/utils.sh

echo -e "\nshowing the version.json..."
cat $repo/version.json
echo -e "\nshowing the metadata.json..."
cat $repo/metadata.json
pr_id=$(cat $repo/pr_id)
echo -e "\nthis pr_id: ${pr_id}\n"
# install zip
apt-get update
apt-get install zip -y

# install gh
wget -qq https://github.com/cli/cli/releases/download/v2.27.0/gh_2.27.0_linux_amd64.tar.gz
tar -xzf gh_2.27.0_linux_amd64.tar.gz -C /usr/local
export PATH="/usr/local/gh_2.27.0_linux_amd64/bin:$PATH"

gh version

cd $repo
echo -e "\n$ git log -n 2"
git log -n 2
prNum=${pr_id}
echo ${pr_id}
#find file
diffFiles=$(gh pr diff ${pr_id} --name-only | grep "^alicloud/" | grep ".go$" | grep -v ".markdown$")
docsFiles=$(gh pr diff ${pr_id} --name-only | grep ".markdown$")
changeFiles=(${docsFiles[@]} ${diffFiles[@]})

if [[ ${#changeFiles[@]} -eq 0 ]]; then
  echo -e "\033[33m[WARNING]\033[0m the pr ${prNum} does not change provider code and there is no need to check."
  exit 0
fi

exampleCount=0
noNeedRun=true
#check if need run
echo "${changeFiles}"
for fileName in ${changeFiles[@]}; do

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

echo $(gh pr checks ${prNum})

exampleCheck=$(gh pr checks ${prNum} | grep "^ExampleTest")

if [[ ${exampleCheck} == "" ]]; then
  echo -e "\033[31m the pr ${prNum} missing ExampleTest action checks and please checking it.\033[0m"
  exit 0
else
  arrIN=(${exampleCheck//"actions"/ })
  ossObjectPath="github-actions"${arrIN[${#arrIN[@]} - 1]}
  echo "exampleCheck result: ${exampleCheck}"
  echo "ossObjectPath: ${ossObjectPath}"
  exampleCheckFail=$(echo ${exampleCheck} | grep "pass")
  if [[ ${exampleCheckFail} != "" ]]; then
    echo -e "\033[32m the pr ${prNum} latest job has passed.\033[0m"
    exit 0
  fi
  exampleCheckFail=$(echo ${exampleCheck} | grep "fail")
  if [[ ${exampleCheckFail} != "" ]]; then
    echo -e "\033[33m the pr ${prNum} latest job has finished, but failed!\033[0m"
    exit 1
  fi
  exampleCheckPending=$(echo ${exampleCheck} | grep "pending")
  #  以下为实际运行
  if [[ ${exampleCheckPending} != "" ]]; then
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
      echo -e "\033[31m uploading the pr ${prNum} example check result log  to oss failed, please checking it.\033[0m"
      exit 1
    fi
    aliyun oss cp ${exampleTerraformLog} oss://${OSS_BUCKET_NAME}/${ossObjectPath}/${exampleTerraformLog} -f --access-key-id ${ALICLOUD_ACCESS_KEY} --access-key-secret ${ALICLOUD_SECRET_KEY} --region ${OSS_BUCKET_REGION}
    if [[ "$?" != "0" ]]; then
      echo -e "\033[31m uploading the pr ${prNum} example check log  to oss failed, please checking it.\033[0m"
      exit 1
    fi
    aliyun oss cp ${exampleTerraformErrorLog} oss://${OSS_BUCKET_NAME}/${ossObjectPath}/${exampleTerraformErrorLog} -f --access-key-id ${ALICLOUD_ACCESS_KEY} --access-key-secret ${ALICLOUD_SECRET_KEY} --region ${OSS_BUCKET_REGION}
    if [[ "$?" != "0" ]]; then
      echo -e "\033[31m uploading the pr ${prNum} example check error log  to oss failed, please checking it.\033[0m"
      exit 1
    fi
  fi
fi
