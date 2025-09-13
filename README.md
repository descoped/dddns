# dddns

Descoped Dynamic DNS is an application provided as a CLI that updates A Records in AWS.

Flow:

* Check Public IP
  * If determined IP is a proxy address, then cancel operation (guard against updating proxy ip) 
* 
