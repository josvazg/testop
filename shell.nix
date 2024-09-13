{ pkgs ? import <nixpkgs> {} }:
with pkgs;
mkShell {
    buildInputs = [
        bash
        go
        kind
        kubectl
	    kubebuilder
    ];
}
