#!/usr/bin/env bash
cur=$(dirname "$(readlink -f $0)")

set -e
BUILD_LOCAL="${BUILD_LOCAL:-"true"}"

SN_ID=1002
SNUSER_ID=1003

source ${cur}/common.sh $1
project_dir=${cur}../../../
read var_pipeline_version var_image_version < <(get_version)

read var_pipeline_version var_image_version < <(get_version)

echo "build with image_version='${var_image_version}' pipeline_version='${var_pipeline_version}'"

if [[ $app == "aaa.bbb" ]]; then
  cd  ${cur}/../
  bash compile.sh
  cd -
fi

var_image_prefix=""
var_service_image_prefix=""

var_base_image_full=""
var_service_image_full="${var_service_image_prefix}/${var_service_image_name}:${var_image_version}"

docker pull ${var_base_image_full}
echo "pulled base image base_image_full='${var_base_image_full}'"

#镜像构建
if [ "${BUILD_LOCAL}" != "true" ]; then
  rm -rf ${codeRootDir}/yuanrong
  tar -xf ${codeRootDir}/yuanrong_${architecture}/*Software_EulerOS_yuanrong.tar.gz -C ${cur}
fi
cd ${cur}

docker build --no-cache --build-arg docker_image=${var_base_image_full} \
                        --build-arg app=${app} \
                        --build-arg SNUSER_ID="${SNUSER_ID}" \
                        --build-arg SN_ID="${SN_ID}" \
                        -t ${var_service_image_full} -f ${cur}/dockerfile/Dockerfile-${app} .

echo "show docker images after build"
docker images "${var_service_image_full}" || true
docker inspect ${var_service_image_full} || true
docker push ${var_service_image_full} || true

write_buildInfo ${var_pipeline_version} ${var_service_image_full}
write_buildImage ${cur} ${var_base_image_full}
