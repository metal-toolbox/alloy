#!/bin/bash

echo "Welcome to Alloy Inband container"

alloy inband

cat <<"EOF" > /statedir/cleanup.sh
#!/usr/bin/env bash
echo "This is the cleanup script.sh"
for i in {1..10}
do
  echo $i
  sleep 1
done
reboot
EOF
chmod +x /statedir/cleanup.sh

echo "task.sh is now finished, cleanup.sh will reboot after 10s"
