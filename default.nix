with import <nixpkgs> {};

stdenv.mkDerivation {
name = "go-env";

buildInputs = [
		go
		syft
		grype
		docker
		docker-credential-helpers
		trivy
];

SOURCE_DATE_EPOCH = 315532800;
PROJDIR = "${toString ./.}";
S_NETWORK="weave";
S_VOLUME_RO_1="/home/$USER/.mesos";

shellHook = ''
		export LD_LIBRARY_PATH="${pkgs.stdenv.cc.cc.lib}/lib"
		export PATH=/tmp/bin:$PATH
		export GOTMPDIR=/tmp
		export TMPDIR=/tmp
		mkdir /tmp/bin
		'';
}
