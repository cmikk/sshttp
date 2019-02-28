sshttp: Simple SSH http/https proxy.
====================================

sshttp is a simple proxy for HTTP and HTTPS sessions over an SSH tunnel,
useful for software which respects the ''http_proxy'' and ''https_proxy''
environment variables, but cannot use the SOCKS proxying built into the
stock openssh client.

Only ssh-agent authentication is supported.

Usage
-----
sshttp may be used to provide a proxy environment for one command:

	sshttp user@host command args ...

or it may be used to set up an environment, in the style of ssh-agent:

	eval $(sshttp user@host)

The running proxy in the latter setup can be killed with:

	sshttp -kill

An existing sshttp proxy can be used conveniently with:

	sshttp -query command args...

or used to set up proxy environment variables with:

	eval $(sshttp -query)
