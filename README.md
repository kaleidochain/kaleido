## Kaleido

Official implementation of the Kaleido chain.

[![API Reference](
https://camo.githubusercontent.com/915b7be44ada53c290eb157634330494ebe3e30a/68747470733a2f2f676f646f632e6f72672f6769746875622e636f6d2f676f6c616e672f6764646f3f7374617475732e737667
)](https://godoc.org/github.com/kaleidochain/kaleido)
[![Go Report Card](https://goreportcard.com/badge/github.com/kaleidochain/kaleido)](https://goreportcard.com/report/github.com/kaleidochain/kaleido)

Automated builds are available for stable releases and the unstable master branch.


Binary archives are published by docker hub, you can fetch or update it by running

```bash
docker pull kaleidochain/kalgo
```

If you want to get full suite of utilities, run

```bash
docker pull kaleidochain/all
```

## Running kalgo

You can refer to our [docs](https://docs.kaleidochain.io) to run a kalgo node. Here we
enumerate a few common parameter combos to get you up to speed quickly on how you can run your
own kalgo instance.

### Docker quick start

One of the quickest ways to get Kaleido up and running on your machine is by using Docker:

```bash
docker run -d --name kalnode -v $HOME/kaleido:/root/.kaleido \
           -p 8545:8545 -p 38883:38883 -p 38883:38883/udp \
           kaleidochain/kalgo --rpc --rpcaddr 0.0.0.0
```

This will start kalgo in fast-sync mode with a DB memory allowance of 1GB just as the above command does.  It will also create a persistent volume in your home directory for saving your blockchain as well as map the default ports.

Do not forget `--rpcaddr 0.0.0.0`, if you want to access RPC from other containers and/or hosts. By default, `kalgo` binds to the local interface and RPC endpoints is not accessible from the outside.

If you want to connect to test network instead of main network, just add `--testnet` at tail.

### Full node on the Kaleido main network

By far the most common scenario is people wanting to simply interact with the Kaleido main network:
create accounts; transfer funds; deploy and interact with contracts. For this particular use-case
the user doesn't care about years-old historical data, so we can fast-sync quickly to the current
state of the network. To do so:

```
$ kalgo --verbosity 0 console
```

This command will:

 * Start kalgo in fast sync mode (default, can be changed with the `--syncmode` flag), causing it to
   download more data in exchange for avoiding processing the entire history of the Kaleido network.
 * Disable all levels log output by `--verbosity 0`
 * Start up kalgo's built-in interactive JavaScript console(via the trailing `console` subcommand)
   through which you can invoke all official `web3` methods as well as kalgo's own management APIs.
   This tool is optional and if you leave it out you can always attach to an already running kalgo instance
   with `kalgo attach`.

### Full node on the Kaleido test network

Transitioning towards developers, if you'd like to play around with creating contracts, you
almost certainly would like to do that without any real money involved until you get the hang of the
entire system. In other words, instead of attaching to the main network, you want to join the **test**
network with your node, which is fully equivalent to the main network, but with play-Kal only.

```
$ kalgo --verbosity 0 --testnet console
```

The `console` subcommand have the exact same meaning as above and they are equally useful on the
testnet too. Please see above for their explanations if you've skipped to here.

Specifying the `--testnet` flag however will reconfigure your kalgo instance a bit:

 * Instead of using the default data directory (`~/.kaleido` on Linux for example), kalgo will nest
   itself one level deeper into a `testnet` subfolder (`~/.kaleido/testnet` on Linux). Note, on OSX
   and Linux this also means that attaching to a running testnet node requires the use of a custom
   endpoint since `kalgo attach` will try to attach to a production node endpoint by default. E.g.
   `kalgo attach <datadir>/testnet/kalgo.ipc`. Windows users are not affected by this.
 * Instead of connecting the Kaleido main network, the client will connect to the test network,
   which uses different P2P bootnodes, different network IDs and genesis states.
   
*Note: Although there are some internal protective measures to prevent transactions from crossing
over between the main network and test network, you should make sure to always use separate accounts
for play-money and real-money. Unless you manually move accounts, kalgo will by default correctly
separate the two networks and will not make any accounts available between them.*

### Configuration

As an alternative to passing the numerous flags to the `kalgo` binary, you can also pass a configuration file via:

```
$ kalgo --config /path/to/your_config.toml
```

To get an idea how the file should look like you can use the `dumpconfig` subcommand to export your existing configuration:

```
$ kalgo --your-favourite-flags dumpconfig
```

## Building the source

Building the source requires both a Go (version 1.12 or later), a C compiler, boost library and autoconf tools.
You can install them using your favourite package manager.
Our core engineering team uses Linux and OSX, so both environments are well supported for development.

Once the dependencies are installed, initial environment setup

```bash
mkdir -p ${GOPATH}/src/github.com/kaleidochain
cd ${GOPATH}/src/github.com/kaleidochain
git clone https://github.com/kaleidochain/kaleido
cd kaleidochain
make deplibs
```

and then run

    make kalgo

or, to build the full suite of utilities:

    make all

## Executables

The kaleido project comes with several executables found in the `cmd` directory.

| Command    | Description |
|:----------:|-------------|
| **`kalgo`** | Our main kaleido client. It is the entry point into the Kaleido network (main-, test- or private net), capable of running as a full node (default), archive node (retaining all historical state) or a light node (retrieving data live). It can be used by other processes as a gateway into the Kaleido network via JSON RPC endpoints exposed on top of HTTP, WebSocket and/or IPC transports. `kalgo --help` for command line options. |
| `kalabigen` | Source code generator to convert contract definitions into easy to use, compile-time type-safe Go packages. It operates on plain contract ABIs with expanded functionality if the contract bytecode is also available. However it also accepts Solidity source files, making development much more streamlined. |
| `bootnode` | Stripped down version of our Kaleido client implementation that only takes part in the network node discovery protocol, but does not run any of the higher level application protocols. It can be used as a lightweight bootstrap node to aid in finding peers in private networks. |
| `evm` | Developer utility version of the EVM (Ethereum Virtual Machine) that is capable of running bytecode snippets within a configurable environment and execution mode. Its purpose is to allow isolated, fine-grained debugging of EVM opcodes (e.g. `evm --code 60ff60ff --debug`). |

## Contribution

Thank you for considering to help out with the source code! We welcome contributions from
anyone on the internet, and are grateful for even the smallest of fixes!

If you'd like to contribute to kaleido, please fork, fix, commit and send a pull request
for the maintainers to review and merge into the main code base.

Please make sure your contributions adhere to our coding guidelines:

 * Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting) guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
 * Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary) guidelines.
 * Pull requests need to be based on and opened against the `master` branch.
 * Commit messages should be prefixed with the package(s) they modify.
   * E.g. "rpc: make trace configs optional"

## License

The kaleido library (i.e. all code outside of the `cmd` directory) is licensed under the
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html), also
included in our repository in the `COPYING.LESSER` file.

The kaleido binaries (i.e. all code inside of the `cmd` directory) is licensed under the
[GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also included
in our repository in the `COPYING` file.
