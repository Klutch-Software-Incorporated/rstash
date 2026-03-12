import "dotenv/config";
import * as pulumi from "@pulumi/pulumi";
import * as digitalocean from "@pulumi/digitalocean";
import * as command from "@pulumi/command";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";

const sshKeyName = process.env.SSH_KEY_NAME;
if (!sshKeyName) {
    throw new Error("SSH_KEY_NAME environment variable is required");
}
const sshKey = digitalocean.getSshKey({ name: sshKeyName });

const project = new digitalocean.Project("rstash-cloud", {
    name: "rstash-cloud",
    purpose: "Static marketing site",
    environment: "Production",
    description: "Infrastructure for rstash.cloud",
    isDefault: false,
});

const droplet = new digitalocean.Droplet("rstash-cloud-droplet", {
    image: "ubuntu-24-04-x64",
    region: "nyc3",
    size: "s-1vcpu-512mb-10gb",
    backups: false,
    sshKeys: [sshKey.then(k => k.id.toString())],
    tags: ["rstash-cloud"],
});

const domain = new digitalocean.Domain("rstash.cloud", {
    name: "rstash.cloud",
});

new digitalocean.DnsRecord("rstash.cloud-@", {
    domain: domain.id,
    type: digitalocean.RecordType.A,
    name: "@",
    value: droplet.ipv4Address,
    ttl: 300,
});

new digitalocean.Firewall("rstash-cloud-fw", {
    dropletIds: [droplet.id.apply(id => parseInt(id, 10))],
    inboundRules: [
        { protocol: "tcp", portRange: "22", sourceAddresses: ["0.0.0.0/0", "::/0"] },
        { protocol: "tcp", portRange: "80", sourceAddresses: ["0.0.0.0/0", "::/0"] },
        { protocol: "tcp", portRange: "443", sourceAddresses: ["0.0.0.0/0", "::/0"] },
    ],
    outboundRules: [
        { protocol: "tcp", portRange: "all", destinationAddresses: ["0.0.0.0/0", "::/0"] },
        { protocol: "udp", portRange: "all", destinationAddresses: ["0.0.0.0/0", "::/0"] },
    ],
});

new digitalocean.ProjectResources("rstash-cloud-resources", {
    project: project.id,
    resources: [droplet.dropletUrn, domain.domainUrn],
});

// --- Server provisioning via SSH ---

const privateKeyPath = process.env.SSH_PRIVATE_KEY_PATH
    ?? path.join(os.homedir(), ".ssh", "id_ed25519");
const privateKey = fs.readFileSync(privateKeyPath, "utf8");

const connection: command.types.input.remote.ConnectionArgs = {
    host: droplet.ipv4Address,
    user: "root",
    privateKey,
};

const infraDir = path.resolve(__dirname, "..");
const containersDir = path.join(infraDir, "containers");

// Copy Caddyfile
const copyCaddyfile = new command.remote.CopyToRemote("copy-caddyfile", {
    connection,
    source: new pulumi.asset.FileAsset(path.join(containersDir, "caddy", "Caddyfile")),
    remotePath: "/tmp/Caddyfile",
}, { dependsOn: [droplet] });

// Copy Quadlet files
const copyCaddyContainer = new command.remote.CopyToRemote("copy-caddy-container", {
    connection,
    source: new pulumi.asset.FileAsset(path.join(containersDir, "caddy", "caddy.container")),
    remotePath: "/tmp/caddy.container",
}, { dependsOn: [droplet] });

const copyCaddyDataVolume = new command.remote.CopyToRemote("copy-caddy-data-volume", {
    connection,
    source: new pulumi.asset.FileAsset(path.join(containersDir, "caddy", "caddy-data.volume")),
    remotePath: "/tmp/caddy-data.volume",
}, { dependsOn: [droplet] });

const copyCaddyConfigVolume = new command.remote.CopyToRemote("copy-caddy-config-volume", {
    connection,
    source: new pulumi.asset.FileAsset(path.join(containersDir, "caddy", "caddy-config.volume")),
    remotePath: "/tmp/caddy-config.volume",
}, { dependsOn: [droplet] });

// Copy and run setup script
const copySetupScript = new command.remote.CopyToRemote("copy-setup-sh", {
    connection,
    source: new pulumi.asset.FileAsset(path.join(infraDir, "setup.sh")),
    remotePath: "/tmp/setup.sh",
}, { dependsOn: [droplet] });

new command.remote.Command("run-setup", {
    connection,
    create: "chmod +x /tmp/setup.sh && /tmp/setup.sh",
}, { dependsOn: [
    copyCaddyfile,
    copyCaddyContainer, copyCaddyDataVolume, copyCaddyConfigVolume,
    copySetupScript,
] });

export const ip = droplet.ipv4Address;
export const siteUrl = pulumi.interpolate`https://rstash.cloud`;
