package client

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/crypto/sha3"
)

type Config struct {
	PackageID  string   `json:"package_id"`
	PrivateKey string   `json:"private_key"`
	Bootstrap  []string `json:"bootstrap"`
	DataDir    string   `json:"data_dir"`
	CacheDir   string   `json:"cache_dir"`
}

func NewConfig() *Config {
	config := &Config{}
	return config
}

func (c *Config) Load(buf []byte) (err error) {
	if err = json.Unmarshal(buf, c); err != nil {
		err = fmt.Errorf("failed to parse config: %v", err)
	}
	return
}

func (c *Config) GetPrivateKey() (privKey crypto.PrivKey, err error) {
	privBytes, err := hex.DecodeString(c.PrivateKey)
	if err != nil {
		err = fmt.Errorf("failed to decode private key: %v", err)
		return
	} else if len(privBytes) != 32 {
		err = fmt.Errorf("illegal private key")
		return
	}
	nested := 0
	switch runtime.GOOS {
	case "darwin":
		nested++
		fallthrough
	case "ios":
		nested++
		fallthrough
	case "android":
		nested++
		fallthrough
	case "linux":
		nested++
		fallthrough
	case "windows":
		privKey = (*crypto.Secp256k1PrivateKey)(secp256k1.PrivKeyFromBytes(privBytes))
		for ; nested > 0; nested-- {
			pkey, e := secp256k1.GeneratePrivateKeyFromRand(bytes.NewBuffer(privBytes))
			if e != nil {
				err = fmt.Errorf("failed to generate drived private key: %v", e)
				return
			}
			privKey = (*crypto.Secp256k1PrivateKey)(pkey)
			privBytes, _ = privKey.Raw()
		}
	default:
		err = fmt.Errorf("unknown system")
	}
	return
}

func (c *Config) GetEthereumAddress() (address string, err error) {
	privBytes, err := hex.DecodeString(c.PrivateKey)
	if err != nil {
		err = fmt.Errorf("failed to decode private key: %v", err)
		return
	}

	curve := secp256k1.S256()

	byteLen := (curve.Params().BitSize + 7) / 8

	pubBytes := make([]byte, 1+2*byteLen)
	pubBytes[0] = 4 // uncompressed point

	x, y := curve.ScalarBaseMult(privBytes)
	x.FillBytes(pubBytes[1 : 1+byteLen])
	y.FillBytes(pubBytes[1+byteLen : 1+2*byteLen])

	hash := sha3.NewLegacyKeccak256()
	hash.Write(pubBytes[1:])
	pubKeyHash := hash.Sum(nil)[12:]
	hash.Reset()

	const addrLength = 20

	hashLength := len(pubKeyHash)
	if hashLength > addrLength {
		pubKeyHash = pubKeyHash[hashLength-addrLength:]
	}

	addrBytes := make([]byte, addrLength)
	copy(addrBytes[addrLength-hashLength:], pubKeyHash)

	var buf [addrLength*2 + 2]byte
	copy(buf[:2], "0x")
	hex.Encode(buf[2:], addrBytes[:])

	// compute checksum
	hash.Write(buf[2:])
	addrHash := hash.Sum(nil)
	for i := 2; i < len(buf); i++ {
		hashByte := addrHash[(i-2)/2]
		if i%2 == 0 {
			hashByte = hashByte >> 4
		} else {
			hashByte &= 0xf
		}
		if buf[i] > '9' && hashByte > 7 {
			buf[i] -= 32
		}
	}

	address = string(buf[:])
	return
}

func (c *Config) GetBootstrap() (addrInfos []peer.AddrInfo, err error) {
	for _, addr := range c.Bootstrap {
		addrInfo, e := peer.AddrInfoFromString(addr)
		if e != nil {
			err = fmt.Errorf("failed to parse peer address: %v", e)
			return
		}
		addrInfos = append(addrInfos, *addrInfo)
	}
	return
}

func (c *Config) GetSDKDir() (dir string, err error) {
	if c.DataDir != "" {
		dir = filepath.Join(c.DataDir, ".aks_sdk")
	} else {
		dir, err = os.UserHomeDir()
		if err != nil {
			err = fmt.Errorf("failed to get home directory: %v", err)
			return
		}
		dir = filepath.Join(dir, ".aks_sdk")
	}
	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		err = fmt.Errorf("failed to create directory: %v", err)
	}
	return
}

func (c *Config) GetImageDir() (dir string, err error) {
	dir, err = c.GetSDKDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "image")
	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		err = fmt.Errorf("failed to create directory: %v", err)
	}
	return
}

func (c *Config) GetDatabaseDir() (dir string, err error) {
	dir, err = c.GetSDKDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "database")
	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		err = fmt.Errorf("failed to create directory: %v", err)
	}
	return
}
