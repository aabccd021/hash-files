{

  nixConfig.allow-import-from-derivation = false;

  inputs.nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  inputs.treefmt-nix.url = "github:numtide/treefmt-nix";

  outputs =
    { self, ... }@inputs:
    let
      lib = inputs.nixpkgs.lib;

      collectInputs =
        is:
        pkgs.linkFarm "inputs" (
          builtins.mapAttrs (
            name: i:
            pkgs.linkFarm name {
              self = i.outPath;
              deps = collectInputs (lib.attrByPath [ "inputs" ] { } i);
            }
          ) is
        );

      overlays.default = (
        final: prev: {
          hash-files = final.writeShellApplication {
            name = "hash-files";
            runtimeInputs = [ final.jq ];
            text = builtins.readFile ./hash-files.sh;
          };
        }
      );

      pkgs = import inputs.nixpkgs {
        system = "x86_64-linux";
        overlays = [ overlays.default ];
      };

      treefmtEval = inputs.treefmt-nix.lib.evalModule pkgs {
        projectRootFile = "flake.nix";
        programs.nixfmt.enable = true;
        programs.prettier.enable = true;
        programs.shfmt.enable = true;
        programs.shellcheck.enable = true;
        settings.formatter.shellcheck.options = [
          "-s"
          "sh"
        ];
        settings.global.excludes = [ "LICENSE" ];
      };

      devShells.default = pkgs.mkShellNoCC {
        buildInputs = [ pkgs.nixd ];
      };

      formatter = treefmtEval.config.build.wrapper;

      packages = devShells // {
        formatting = treefmtEval.config.build.check self;
        formatter = formatter;
        allInputs = collectInputs inputs;
        hash-files = pkgs.hash-files;
        default = pkgs.hash-files;
      };

    in
    {

      packages.x86_64-linux = packages // {
        gcroot = pkgs.linkFarm "gcroot" packages;
      };

      checks.x86_64-linux = packages;
      formatter.x86_64-linux = formatter;
      devShells.x86_64-linux = devShells;
      overlays = overlays;

    };
}
