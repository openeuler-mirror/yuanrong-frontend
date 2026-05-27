#!/bin/bash
# Copyright (c) Huawei Technologies Co., Ltd. 2022-2023. All rights reserved.
set -e

typeset bashrc_file="${SNHOME}/.connect_bashrc"
if [ ! -f "${bashrc_file}" ];then
 echo "" > "${bashrc_file}"
fi
#delete
sed -i "/alias log=/d" "${bashrc_file}"
sed -i "/alias logd=/d" "${bashrc_file}"
sed -i "/alias logk=/d" "${bashrc_file}"
sed -i "/alias tflogfc=/d" "${bashrc_file}"
sed -i "/alias tflogff=/d" "${bashrc_file}"
sed -i "/alias tflogfs=/d" "${bashrc_file}"
sed -i "/alias tflogr=/d" "${bashrc_file}"
sed -i "/alias tflogstd=/d" "${bashrc_file}"
sed -i "/alias tflogu=/d" "${bashrc_file}"
sed -i "/alias tflogsms=/d" "${bashrc_file}"
sed -i "/alias tflogsts=/d" "${bashrc_file}"
sed -i "/alias tflogfa=/d" "${bashrc_file}"
sed -i "/alias tflogrm=/d" "${bashrc_file}"
sed -i "/alias bin=/d" "${bashrc_file}"
sed -i "/alias l=/d" "${bashrc_file}"
#append
sed -i "\$a\alias log='cd ${HOME}/log'" "${bashrc_file}"
sed -i "\$a\alias logd='cd ${HOME}/log'" "${bashrc_file}"
sed -i "\$a\alias logk='cd ${SNHOME}/log'" "${bashrc_file}"
sed -i "\$a\alias tflogfc='tail -f ${HOME}/log/faascontroller-run.*.log'" "${bashrc_file}"
sed -i "\$a\alias tflogff='tail -f ${HOME}/log/faasfrontend-run.*.log'" "${bashrc_file}"
sed -i "\$a\alias tflogfs='tail -f ${HOME}/log/faasscheduler-run.*.log'" "${bashrc_file}"
sed -i "\$a\alias tflogr='tail -f ${HOME}/log/runtime-go-run.*.log'" "${bashrc_file}"
sed -i "\$a\alias tflogstd='tail -f ${HOME}/log/*user_func_std.log'" "${bashrc_file}"
sed -i "\$a\alias tflogu='tail -f ${HOME}/log/urma.log'" "${bashrc_file}"
sed -i "\$a\alias tflogsms='tail -f ${HOME}/log/sms.sdk.log'" "${bashrc_file}"
sed -i "\$a\alias tflogsts='tail -f ${HOME}/log/sts.sdk.log'" "${bashrc_file}"
sed -i "\$a\alias tflogfa='tail -f ${SNHOME}/log/*function_agent.log'" "${bashrc_file}"
sed -i "\$a\alias tflogrm='tail -f ${SNHOME}/log/*runtime_manager.log'" "${bashrc_file}"
sed -i "\$a\alias bin='cd ${SNHOME}/bin'" "${bashrc_file}"
sed -i "\$a\alias l='ls -la --color=auto'" "${bashrc_file}"
sed -i "\$a\HISTFILE=${SNHOME}/.bash_history" "${bashrc_file}"
sed -i "\$a\export HISTFILE" "${bashrc_file}"
sed -i '/source ${SNHOME}\/.connect_bashrc/d' "${SNHOME}"/.bashrc
sed -i "\$a\source ${SNHOME}\/.connect_bashrc" "${SNHOME}"/.bashrc
sed -i '/PATH/d' "${SNHOME}/.bashrc"
cp -a "${SNHOME}"/.bashrc "${HOME}"/.bashrc
source "${SNHOME}"/.bashrc
####################################### end update .bashrc #############################################