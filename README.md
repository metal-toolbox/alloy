Alloy collects and reports hardware inventory inband.

```
❯ ./alloy inventory 
NAME:
   alloy inventory - collect inventory

USAGE:
   alloy inventory [command options] [arguments...]

OPTIONS:
   --component-type value, -t value  Component slug to collect inventory for.
   --server-url value, -u value      server URL to submit inventory. [$SERVER_URL]
   --local-file value, -l value      write inventory results to local file.
   --dry-run, -d                     collect inventory, skip posting data to server URL.
   --verbose, -v                     Turn on verbose messages for debugging.
   
2022/05/13 16:37:29 Required flag "server-url" not set

```