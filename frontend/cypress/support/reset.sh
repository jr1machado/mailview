#!/bin/bash

pkill -9 listmonk
 cd ../
./mailview --install --yes
./mailview > /dev/null 2>/dev/null &
