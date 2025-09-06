# Overview
perfAnalyzer can monitor the following Components:
- CPU
- RAM
- Network Cards
- Disks

It has the ability to monitor multiple instances of one Component.

## Usage
```
perfAnalyzer -n enp1s0 -d sda -c -r -i 2000 -cmd 'cp /some/random/file /to/some/random/path'
```
**Options:**
<pre>
	-n <device>   monitor specified Network Device\
 	-d <device>   monitor specified Disk\
	-c            monitor CPU\
	-r            monitor RAM\
	-i            sampling rate in ms (default:1000)\
	-cmd          execute command while monitoring with perfAnalyzer\
</pre>

