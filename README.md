# Raft Starter

This project uses the Hashicorp Raft implementation to easily and robustly start a new service.

It is intentionally minimalistic and it was based on the example found at https://github.com/otoolep/hraftd with some improvements out of which these are the most notable:

- Switched to cluster defintion in advance by specifiying peers (handy in Kubernetes)
- Removed all join functionality as it was not needed when knowing peers in advance
- Reading env vars instead of program arguments (allows for easier customization while ran in Kubernetes)
- Added proper defaults for all configuration variables
  - `NODE_ID` is by default the system hostname (very handy when ran in Kubernetes)
  - `STATE_DIR` defaults to `/tmp/<NODE_ID>`
  - `KV_STORE` defaults to `boltdb` but can be also `memory` if you need more speed at the price of not storing your data at all on disk
  - `HTTP_ADDR` defaults to `0.0.0.0`
  - `HTTP_PORT` defaults to `11000`
  - `RAFT_ADDR` defaults to `0.0.0.0`
  - `RAFT_PORT` defaults to `12000`
  - `RAFT_ADVERTISE_ADDR` defaults to the IPV4 which is obtain from resolving the system hostname (also handy when ran in Kubernetes)
  - `RAFT_ADVERTISE_PORT` defaults be the same as `RAFT_PORT`
  - `PEERS` of the form `node1@10.1.1.1:12000,node2@10.1.1.2:12000,...` defaults to be `<NODE_ID>@<RAFT_ADVERTISE_ADDR>:<RAFT_ADVERTISE_PORT>` which sets up a one node cluster
- All these defaults mean that you can run a single node without any environment or argument configuration
- State directory is not used for snapshots when selecting the memory key value store
- Storing the key values as `map[string]interface{}` instead of `map[string]string`
  - When retrieving an inexistent key it returns JSON null instead of ""
  - Supports numbers
  - Supports arrays
  - Supports dictionaries
- Separated each responsibility in one Go file
- Cleanly commented `main.go`
- Made sure all possible errors are accounted for
- Added correct signal support
- Used the [Hashicorp logger](https://github.com/hashicorp/go-hclog)
- Added API to the Store and an example of how to detect Raft state changes
- Added Procfile for easier debug using [goreman](https://github.com/mattn/goreman) which is meant to be run like this:  
        `go build -o ./tmp/app && goreman start`
- Added example of how to run it in Kubernetes:  
        `./_scripts/build.sh`  
        `./_scripts/install.sh`  
  and when you're done with it:  
        `./_scripts/uninstall.sh`  

Have fun and make as many of your services highly available 🖖  
Adrian Punga
