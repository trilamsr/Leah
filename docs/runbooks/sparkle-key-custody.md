# Sparkle EdDSA key custody

Leah uses Sparkle 2.x EdDSA (ed25519) signatures for update verification.
The private key signs every release ZIP; Sparkle verifies against the public
key embedded in `Info.plist` (`SUPublicEDKey`).

Loss of the private key means no future signed updates can be delivered — users
would need to reinstall manually. Three-place backup is mandatory.

## Generate the keypair (one-time)

```sh
scripts/release/generate-sparkle-keys.sh
```

The script locates `generate_keys` from the SPM `.build` output (build the
app first: `cd app/Leah && swift build`). It writes the private key to the
login Keychain and prints the public key to stdout.

After running:
1. Copy the `SUPublicEDKey` value into `app/Leah/Sources/LeahApp/Info.plist`.
2. Complete all three backup locations below before doing anything else.

## Three-place private key backup

### 1. 1Password vault

- Vault: **Personal** (or team vault if shared operations)
- Item name: `Leah EdDSA private key`
- Field: `private_key` — paste the base64-encoded private key string
- Store alongside: public key, date generated, machine hostname

Retrieve: open 1Password → search "Leah EdDSA" → copy `private_key` field.

### 2. age-encrypted file on Time Machine volume

```sh
# Encrypt (requires age: brew install age)
age-keygen -o /tmp/age-backup.key          # generate age identity key, store it in 1Password too
age -r "$(cat /tmp/age-backup.key | grep 'public key' | awk '{print $NF}')" \
    -o ~/Documents/leah-sparkle-private.age \
    <(security find-generic-password -a Leah -s "ed25519" -w)
```

Store `leah-sparkle-private.age` in a location covered by Time Machine.
Store the age identity key in 1Password alongside the Sparkle key.

Retrieve:
```sh
age -d -i /path/to/age-backup.key leah-sparkle-private.age
```

### 3. BIP39 mnemonic paper printout

Convert the private key bytes to a 24-word BIP39 mnemonic:

```sh
# Using bip39 CLI (brew install bip39 or https://github.com/trezor/python-mnemonic)
security find-generic-password -a Leah -s "ed25519" -w | base64 -d | bip39
```

Print the 24 words, write the date and "Leah EdDSA signing key" on the page,
laminate or store in a waterproof sleeve in a fireproof safe.

To restore from mnemonic:
```sh
bip39 --restore   # outputs bytes → re-encode base64 → import to Keychain
```

## Rotating the key

Key rotation requires a new signed release carrying both old and new public
keys, or a forced reinstall campaign. Avoid rotation unless the private key is
compromised. See Sparkle docs for the `SUPublicEDKey` migration path.

## Revocation / compromise

If the private key is leaked:
1. Immediately rotate (see above).
2. Post a security advisory at `https://maydow.github.io/leah/security`.
3. Invalidate the compromised key from the Keychain on all machines.
