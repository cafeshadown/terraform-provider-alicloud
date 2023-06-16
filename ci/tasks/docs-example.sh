#!/usr/bin/env bash

: ${ALICLOUD_ACCESS_KEY:?}
: ${ALICLOUD_SECRET_KEY:?}
: ${ALICLOUD_ACCOUNT_ID:?}
: ${DING_TALK_TOKEN:=""}
: ${OSS_BUCKET_NAME:=?}
: ${OSS_BUCKET_REGION:=?}
: ${FC_SERVICE:?}
: ${FC_REGION:?}
: ${GITHUB_TOKEN:?}

repo=terraform-provider-alicloud
export GITHUB_TOKEN=${GITHUB_TOKEN}
export GH_REPO=aliyun/${repo}

echo "hello example"