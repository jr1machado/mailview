#!/bin/bash

pkill -9 listmonk
 cd ../
./MailView --install --yes
./MailView > /dev/null 2>/dev/null &
