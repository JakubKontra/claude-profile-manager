{
  description = "cpm — Claude Profile Manager";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        cpm = pkgs.buildGo126Module {
          pname = "cpm";
          version = "0.6.0";
          src = self;
          vendorHash = "sha256-n58Qmiv3gik1qkuXQFbQ+soeOQtUz1dUocEAJepqp/E=";
          env.CGO_ENABLED = 0;
          # These tests exec generated scripts with a /bin/bash shebang, which the sandbox lacks.
          checkFlags = [ "-skip=TestHookExecution|TestGenerateWrapperExecutes" ];
          ldflags = [
            "-s" "-w"
            "-X github.com/jakubkontra/cpm/internal.Version=0.6.0"
            "-X github.com/jakubkontra/cpm/internal.Commit=${self.shortRev or "dirty"}"
          ];
          meta = {
            description = "Claude Profile Manager — manage multiple Claude Code accounts";
            homepage = "https://github.com/jakubkontra/claude-profile-manager";
            license = pkgs.lib.licenses.mit;
            mainProgram = "cpm";
          };
        };
        default = cpm;
      });
    };
}
