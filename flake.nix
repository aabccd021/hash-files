{

  nixConfig.allow-import-from-derivation = false;

  inputs.nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  inputs.treefmt-nix.url = "github:numtide/treefmt-nix";

  outputs = { self, nixpkgs, treefmt-nix }:
    let

      overlay = (final: prev: {
        hash_files = final.writeShellApplication {
          name = "hash_files";
          runtimeInputs = [ final.jq ];
          text = builtins.readFile ./hash_files.sh;
        };
      });

      pkgs = import nixpkgs {
        system = "x86_64-linux";
        overlays = [ overlay ];
      };

      treefmtEval = treefmt-nix.lib.evalModule pkgs {
        projectRootFile = "flake.nix";
        programs.nixpkgs-fmt.enable = true;
        programs.prettier.enable = true;
        programs.shfmt.enable = true;
        programs.shellcheck.enable = true;
        settings.formatter.shellcheck.options = [ "-s" "sh" ];
        settings.global.excludes = [ "LICENSE" ];
      };

      devShells.default = pkgs.mkShellNoCC {
        buildInputs = [ pkgs.nixd ];
      };

      formatter = treefmtEval.config.build.wrapper;

      packages = {
        formatting = treefmtEval.config.build.check self;
        formatter = formatter;
        hash_files = pkgs.hash_files;
        default = pkgs.hash_files;
      };

    in
    {

      packages.x86_64-linux = packages // devShells // {
        gcroot = pkgs.linkFarm "gcroot" packages;
      };

      checks.x86_64-linux = packages;
      formatter.x86_64-linux = formatter;
      devShells.x86_64-linux = devShells;
      overlays.default = overlay;

    };
}
