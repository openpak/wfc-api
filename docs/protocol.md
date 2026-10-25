# Protocol: wfc-api

Internal HTTP defaults to WFC_HTTP_ADDR=:20209. GET /healthz is unauthenticated;
operator mutations/profile reads retain WFC_API_SECRET role checks.
WFC_INTERNAL_KEY authenticates outbound session/directory calls. Console HTTP reaches this owner
through wfc-nas; TCP/UDP ports belong directly to their protocol's container.
Runtime frontend/backend net/rpc endpoints stay on loopback inside each TCP
service container. Restarting another protocol does not restart these sockets.
