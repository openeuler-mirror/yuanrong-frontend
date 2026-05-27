#!/bin/bash
# Copyright (c) Huawei Technologies Co., Ltd. 2022-2023. All rights reserved.
set -e

component_name=$1

typeset bashrc_file="${HOME}/.connect_bashrc"
if [ ! -f "${bashrc_file}" ];then
 echo "" > "${bashrc_file}"
fi
#delete
sed -i "/alias log=/d" "${bashrc_file}"
sed -i "/alias logf=/d" "${bashrc_file}"
sed -i "/alias tflogr=/d" "${bashrc_file}"
sed -i "/alias bin=/d" "${bashrc_file}"
sed -i "/alias l=/d" "${bashrc_file}"
#append
sed -i "\$a\alias log='cd ${HOME}/log'" "${bashrc_file}"
sed -i "\$a\alias logf='cd ${HOME}/log'" "${bashrc_file}"
sed -i "\$a\alias tflogr='tail -f ${HOME}/log/*${component_name}.log'" "${bashrc_file}"
sed -i "\$a\alias bin='cd ${HOME}/bin'" "${bashrc_file}"
sed -i "\$a\alias l='ls -la --color=auto'" "${bashrc_file}"
sed -i '/source ~\/.connect_bashrc/d' "${HOME}"/.bashrc
sed -i "\$a\source ~\/.connect_bashrc" "${HOME}"/.bashrc
source "${HOME}"/.bashrc
####################################### end update .bashrc #############################################