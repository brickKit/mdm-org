// Command mdm-org runs the organization component alone (serve, migrate); the shells compile in org.Definition().
package main

import (
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/mdm-org/v3/org"
)

func main() { launcher.Main(org.Definition()) }
